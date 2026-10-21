package app

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/authentication"
	"core5g/internal/control"
	"core5g/internal/registration"
	"core5g/internal/routing"
	"core5g/internal/session"
	"core5g/internal/simulator"
	"core5g/internal/store"
	"core5g/internal/subscriber"
	"core5g/pkg/config"
	"core5g/pkg/logging"
	"core5g/pkg/telemetry"
	"core5g/web"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Main(defaultService string) {
	cfg := config.Load(defaultService)
	logging.Configure(cfg.Service)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := Run(ctx, cfg); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}
func Run(ctx context.Context, cfg config.Config) error {
	shutdown, err := telemetry.StartTracing(ctx, cfg.Service)
	if err != nil {
		return err
	}
	defer func() {
		flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(flush)
	}()
	meter := telemetry.New(cfg.Service, cfg.RPCTimeout)
	addresses := map[string]string{"udm": cfg.UDM, "ausf": cfg.AUSF, "amf": cfg.AMF, "smf": cfg.SMF, "upf": cfg.UPF, "gnb-simulator": cfg.GNB}
	dependencies := map[string][]string{"udm": {}, "ausf": {"udm"}, "amf": {"ausf", "smf"}, "smf": {"amf", "upf"}, "upf": {"smf"}, "gnb-simulator": {"amf", "upf"}, "control-api": {"udm", "ausf", "amf", "smf", "upf", "gnb-simulator"}, "ue-simulator": {"gnb-simulator"}}
	needed, valid := dependencies[cfg.Service]
	if !valid {
		return fmt.Errorf("unknown service %q", cfg.Service)
	}
	connections := map[string]*grpc.ClientConn{}
	for _, name := range needed {
		connection, err := grpc.NewClient(addresses[name], grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(meter.Client), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
		if err != nil {
			return err
		}
		defer connection.Close()
		connections[name] = connection
	}
	if cfg.Service == "ue-simulator" {
		return runUE(ctx, cfg, corev1.NewGNBServiceClient(connections["gnb-simulator"]))
	}
	var persistence *store.Store
	if cfg.Service == "udm" || cfg.Service == "amf" || cfg.Service == "smf" || cfg.Service == "upf" {
		if cfg.DatabaseURL == "" {
			persistence = store.NewMemory(cfg.Service)
			slog.Warn("using volatile memory; configure DATABASE_URL for persistence")
		} else {
			dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			persistence, err = store.NewPostgres(dbCtx, cfg.DatabaseURL, cfg.Service)
			cancel()
			if err != nil {
				return err
			}
		}
		defer persistence.Close()
	}
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(meter.Unary), grpc.StatsHandler(otelgrpc.NewServerHandler()))
	healthServer := health.NewServer()
	healthv1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	corev1.RegisterFaultServiceServer(server, &telemetry.FaultService{Meter: meter, Changed: func(failed bool) {
		state := healthv1.HealthCheckResponse_SERVING
		if failed {
			state = healthv1.HealthCheckResponse_NOT_SERVING
		}
		healthServer.SetServingStatus("", state)
	}})
	var api *control.API
	switch cfg.Service {
	case "udm":
		service := &subscriber.Service{Store: persistence}
		if err = service.Seed(ctx, cfg.SeedSubscribers); err != nil {
			return err
		}
		corev1.RegisterUDMServiceServer(server, service)
	case "ausf":
		corev1.RegisterAUSFServiceServer(server, &authentication.Service{UDM: corev1.NewUDMServiceClient(connections["udm"]), Timeout: cfg.RPCTimeout})
	case "amf":
		service := &registration.Service{Store: persistence, AUSF: corev1.NewAUSFServiceClient(connections["ausf"]), SMF: corev1.NewSMFServiceClient(connections["smf"]), Timeout: cfg.RPCTimeout}
		if err = service.Recover(ctx); err != nil {
			return err
		}
		corev1.RegisterAMFServiceServer(server, service)
		meter.Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "core5g_registered_ues", Help: "Currently registered virtual UE."}, func() float64 {
			c, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			list, err := service.ListUEs(c, &corev1.Empty{})
			if err != nil {
				return 0
			}
			count := 0
			for _, ue := range list.Equipments {
				if ue.State == "REGISTERED" || ue.State == "SESSION_ACTIVE" {
					count++
				}
			}
			return float64(count)
		}))
	case "smf":
		service := &session.Service{Store: persistence, AMF: corev1.NewAMFServiceClient(connections["amf"]), UPF: corev1.NewUPFServiceClient(connections["upf"]), Timeout: cfg.RPCTimeout}
		corev1.RegisterSMFServiceServer(server, service)
		go func() {
			timer := time.NewTicker(5 * time.Second)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					reconcileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
					if err := service.Reconcile(reconcileCtx); err != nil {
						slog.Warn("session reconciliation incomplete", "error", err)
					}
					cancel()
				}
			}
		}()
		meter.Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "core5g_active_sessions", Help: "Currently active PDU sessions."}, func() float64 {
			c, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			list, err := service.ListSessions(c, &corev1.SessionFilter{})
			if err != nil {
				return 0
			}
			count := 0
			for _, item := range list.Sessions {
				if item.State == "ACTIVE" {
					count++
				}
			}
			return float64(count)
		}))
	case "upf":
		service := &routing.Service{Store: persistence, SMF: corev1.NewSMFServiceClient(connections["smf"]), Timeout: cfg.RPCTimeout}
		corev1.RegisterUPFServiceServer(server, service)
		for _, result := range []string{"sent", "received", "lost", "rejected"} {
			meter.Registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "core5g_packets_total", Help: "Durable virtual packet counters.", ConstLabels: prometheus.Labels{"result": result}}, func() float64 {
				c, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				stats, err := service.GetStats(c, &corev1.StatsRequest{})
				if err != nil {
					return 0
				}
				switch result {
				case "sent":
					return float64(stats.PacketsSent)
				case "received":
					return float64(stats.PacketsReceived)
				case "lost":
					return float64(stats.PacketsLost)
				default:
					return float64(stats.PacketsRejected)
				}
			}))
		}
	case "gnb-simulator":
		corev1.RegisterGNBServiceServer(server, &simulator.GNB{AMF: corev1.NewAMFServiceClient(connections["amf"]), UPF: corev1.NewUPFServiceClient(connections["upf"])})
	case "control-api":
		api = control.New(ctx, control.Clients{UDM: corev1.NewUDMServiceClient(connections["udm"]), AMF: corev1.NewAMFServiceClient(connections["amf"]), SMF: corev1.NewSMFServiceClient(connections["smf"]), UPF: corev1.NewUPFServiceClient(connections["upf"]), GNB: corev1.NewGNBServiceClient(connections["gnb-simulator"]), Connections: connections}, cfg.RequestTimeout)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(meter.Registry, promhttp.HandlerOpts{Timeout: 3 * time.Second}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ready := !meter.FaultEnabled.Load()
		if persistence != nil {
			check, cancel := context.WithTimeout(r.Context(), time.Second)
			ready = ready && persistence.Ping(check) == nil
			cancel()
		}
		if api != nil {
			for _, service := range api.Statuses(r.Context()) {
				ready = ready && service.Status == "UP"
			}
		}
		if !ready {
			http.Error(w, "not ready", 503)
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	if api != nil {
		mux.Handle("/api/", otelhttp.NewHandler(api.Handler(), "control-api"))
		mux.Handle("/", http.FileServer(http.FS(web.Assets)))
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	errors := make(chan error, 2)
	if cfg.Service != "control-api" {
		listener, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			return err
		}
		go func() {
			if err := server.Serve(listener); err != nil {
				errors <- err
			}
		}()
	}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errors <- err
		}
	}()
	slog.Info("service ready", "http", cfg.HTTPAddr, "grpc", cfg.GRPCAddr)
	select {
	case <-ctx.Done():
	case err = <-errors:
	}
	stop, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(stop)
	done := make(chan struct{})
	go func() { server.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-stop.Done():
		server.Stop()
	}
	return err
}
func runUE(ctx context.Context, cfg config.Config, gnb corev1.GNBServiceClient) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	ueID := config.Env("UE_ID", uuid.NewString())
	result, err := gnb.RegisterUE(ctx, &corev1.RegisterUERequest{RequestId: uuid.NewString(), UeId: ueID, Supi: config.Env("UE_SUPI", "imsi-001010000000001"), GnbId: "gnb-cli-01", Credential: config.Env("UE_CREDENTIAL", "demo-key")})
	if err != nil {
		return err
	}
	if !result.Accepted {
		return fmt.Errorf("registration refused: %s", result.ReasonCode)
	}
	session, err := gnb.CreateSession(ctx, &corev1.CreateSessionRequest{RequestId: uuid.NewString(), UeId: ueID})
	if err != nil {
		return err
	}
	packet, err := gnb.SendPacket(ctx, &corev1.Packet{SessionId: session.SessionId, RequestId: uuid.NewString(), Source: session.IpAddress, Destination: "203.0.113.10", Size: 512, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"ue_id": ueID, "registration": result, "session": session, "packet": packet})
}
