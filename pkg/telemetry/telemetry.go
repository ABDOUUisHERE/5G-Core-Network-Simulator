package telemetry

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type requestKey struct{}
type Meter struct {
	Registry     *prometheus.Registry
	requests     *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	service      string
	Timeout      time.Duration
	FaultEnabled atomic.Bool
	FaultDelay   atomic.Uint32
}

func New(service string, timeout time.Duration) *Meter {
	registry := prometheus.NewRegistry()
	m := &Meter{Registry: registry, service: service, Timeout: timeout}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "core5g_rpc_requests_total", Help: "Completed gRPC requests."}, []string{"service", "method", "code"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "core5g_rpc_duration_seconds", Help: "RPC latency.", Buckets: prometheus.ExponentialBuckets(0.001, 2, 13)}, []string{"service", "method"})
	registry.MustRegister(m.requests, m.duration, prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "core5g_fault_enabled", Help: "Injected unavailability."}, func() float64 {
		if m.FaultEnabled.Load() {
			return 1
		}
		return 0
	}))
	return m
}
func RequestID(ctx context.Context) string {
	if value, ok := ctx.Value(requestKey{}).(string); ok {
		return value
	}
	return uuid.NewString()
}
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestKey{}, id)
}
func (m *Meter) Unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
	started := time.Now()
	id := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get("request-id")
		if len(values) > 0 && len(values[0]) <= 64 {
			id = values[0]
		}
	}
	if id == "" {
		id = uuid.NewString()
	}
	ctx = WithRequestID(ctx, id)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = status.Error(codes.Internal, "internal handler failure")
			slog.Error("handler panic", "method", info.FullMethod)
		}
		code := status.Code(err).String()
		m.requests.WithLabelValues(m.service, info.FullMethod, code).Inc()
		m.duration.WithLabelValues(m.service, info.FullMethod).Observe(time.Since(started).Seconds())
		traceID := trace.SpanContextFromContext(ctx).TraceID().String()
		slog.Info("rpc completed", "method", info.FullMethod, "code", code, "duration_ms", float64(time.Since(started).Microseconds())/1000, "request_id", id, "trace_id", traceID)
	}()
	exempt := strings.Contains(info.FullMethod, "FaultService/") || strings.HasPrefix(info.FullMethod, "/grpc.health.")
	if !exempt {
		if m.FaultEnabled.Load() {
			return nil, status.Error(codes.Unavailable, "dependency failure injected by lab control")
		}
		if delay := m.FaultDelay.Load(); delay > 0 {
			timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, status.FromContextError(ctx.Err()).Err()
			case <-timer.C:
			}
		}
	}
	return handler(ctx, req)
}
func (m *Meter) Client(ctx context.Context, method string, req, reply any, connection *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "request-id", RequestID(ctx))
	return invoke(ctx, method, req, reply, connection, options...)
}

type FaultService struct {
	corev1.UnimplementedFaultServiceServer
	Meter   *Meter
	Changed func(bool)
}

func (s *FaultService) SetFault(_ context.Context, r *corev1.Fault) (*corev1.Fault, error) {
	if r.DelayMs > 30000 {
		return nil, status.Error(codes.InvalidArgument, "fault delay must be <= 30000 ms")
	}
	s.Meter.FaultEnabled.Store(r.Enabled)
	s.Meter.FaultDelay.Store(r.DelayMs)
	if s.Changed != nil {
		s.Changed(r.Enabled)
	}
	return &corev1.Fault{Enabled: r.Enabled, DelayMs: r.DelayMs}, nil
}
func (s *FaultService) GetFault(context.Context, *corev1.Empty) (*corev1.Fault, error) {
	return &corev1.Fault{Enabled: s.Meter.FaultEnabled.Load(), DelayMs: s.Meter.FaultDelay.Load()}, nil
}
func StartTracing(ctx context.Context, service string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", service))))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}
