package control

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/pkg/config"
	"core5g/pkg/telemetry"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

type Clients struct {
	UDM         corev1.UDMServiceClient
	AMF         corev1.AMFServiceClient
	SMF         corev1.SMFServiceClient
	UPF         corev1.UPFServiceClient
	GNB         corev1.GNBServiceClient
	Connections map[string]*grpc.ClientConn
}
type API struct {
	Clients
	Timeout time.Duration
	mu      sync.Mutex
	jobs    map[string]*Job
	root    context.Context
}
type ServiceStatus struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Latency float64 `json:"latency_ms"`
}
type Job struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Count     int      `json:"count"`
	Completed int      `json:"completed"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	P50       float64  `json:"p50_ms"`
	P95       float64  `json:"p95_ms"`
	P99       float64  `json:"p99_ms"`
	Duration  float64  `json:"duration_ms"`
	Errors    []string `json:"errors"`
}

func New(ctx context.Context, clients Clients, timeout time.Duration) *API {
	return &API{Clients: clients, Timeout: timeout, jobs: map[string]*Job{}, root: ctx}
}
func write(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	code := status.Code(err)
	httpCode := 500
	switch code {
	case codes.InvalidArgument:
		httpCode = 400
	case codes.NotFound:
		httpCode = 404
	case codes.AlreadyExists, codes.FailedPrecondition:
		httpCode = 409
	case codes.ResourceExhausted:
		httpCode = 429
	case codes.Unavailable:
		httpCode = 503
	case codes.DeadlineExceeded:
		httpCode = 504
	case codes.Canceled:
		httpCode = 408
	}
	write(w, httpCode, map[string]string{"code": code.String(), "message": status.Convert(err).Message()})
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return status.Error(codes.InvalidArgument, "invalid JSON body or unknown field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return status.Error(codes.InvalidArgument, "one JSON object required")
	}
	return nil
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/overview", a.overview)
	mux.HandleFunc("GET /api/ues", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.AMF.ListUEs(r.Context(), &corev1.Empty{})
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.SMF.ListSessions(r.Context(), &corev1.SessionFilter{UeId: r.URL.Query().Get("ue_id")})
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := a.SMF.GetSession(r.Context(), &corev1.SessionRequest{SessionId: r.PathValue("id")})
		if err != nil {
			fail(w, err)
			return
		}
		counters, counterErr := a.UPF.GetStats(r.Context(), &corev1.StatsRequest{SessionId: item.SessionId})
		result := map[string]any{"session": item, "counters": counters}
		if counterErr != nil {
			result["counter_error"] = status.Code(counterErr).String()
		}
		write(w, 200, result)
	})
	mux.HandleFunc("GET /api/subscribers", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.UDM.ListSubscribers(r.Context(), &corev1.Empty{})
		if err != nil {
			fail(w, err)
			return
		}
		for _, subscriber := range result.Subscribers {
			subscriber.AuthProfile = ""
		}
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/subscribers", func(w http.ResponseWriter, r *http.Request) {
		input := new(corev1.Subscriber)
		if err := decode(w, r, input); err != nil {
			fail(w, err)
			return
		}
		result, err := a.UDM.PutSubscriber(r.Context(), input)
		if err != nil {
			fail(w, err)
			return
		}
		result.AuthProfile = ""
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/ues", a.register)
	mux.HandleFunc("POST /api/ues/{id}/register", a.register)
	mux.HandleFunc("POST /api/ues/{id}/deregister", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.GNB.DeregisterUE(r.Context(), &corev1.UERequest{UeId: r.PathValue("id"), RequestId: telemetry.RequestID(r.Context())})
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		input := new(corev1.CreateSessionRequest)
		if err := decode(w, r, input); err != nil {
			fail(w, err)
			return
		}
		if input.RequestId == "" {
			input.RequestId = telemetry.RequestID(r.Context())
		}
		result, err := a.GNB.CreateSession(r.Context(), input)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 201, result)
	})
	mux.HandleFunc("POST /api/sessions/{id}/release", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.SMF.ReleaseSession(r.Context(), &corev1.SessionRequest{SessionId: r.PathValue("id"), RequestId: telemetry.RequestID(r.Context())})
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/packets", a.packet)
	mux.HandleFunc("POST /api/traffic/config", func(w http.ResponseWriter, r *http.Request) {
		input := new(corev1.TrafficConfig)
		if err := decode(w, r, input); err != nil {
			fail(w, err)
			return
		}
		result, err := a.UPF.ConfigureTraffic(r.Context(), input)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/faults/{service}", func(w http.ResponseWriter, r *http.Request) {
		connection, ok := a.Connections[r.PathValue("service")]
		if !ok {
			fail(w, status.Error(codes.NotFound, "unknown service"))
			return
		}
		input := new(corev1.Fault)
		if err := decode(w, r, input); err != nil {
			fail(w, err)
			return
		}
		result, err := corev1.NewFaultServiceClient(connection).SetFault(r.Context(), input)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, 200, result)
	})
	mux.HandleFunc("POST /api/simulations", a.startJob)
	mux.HandleFunc("GET /api/simulations/{id}", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		job, ok := a.jobs[r.PathValue("id")]
		if !ok {
			fail(w, status.Error(codes.NotFound, "simulation not found"))
			return
		}
		write(w, 200, job)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || origin.Host != r.Host {
				write(w, http.StatusForbidden, map[string]string{"code": "PERMISSION_DENIED", "message": "cross-origin control requests are not allowed"})
				return
			}
		}
		started := time.Now()
		id := r.Header.Get("X-Request-ID")
		parsed, parseErr := uuid.Parse(id)
		if parseErr != nil || parsed == uuid.Nil {
			id = uuid.NewString()
		} else {
			id = parsed.String()
		}
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(telemetry.WithRequestID(r.Context(), id), a.Timeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
		slog.Info("http completed", "request_id", id, "trace_id", trace.SpanContextFromContext(ctx).TraceID().String(), "method", r.Method, "path", r.URL.Path, "duration_ms", float64(time.Since(started).Microseconds())/1000)
	})
}
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	input := new(corev1.RegisterUERequest)
	if err := decode(w, r, input); err != nil {
		fail(w, err)
		return
	}
	if id := r.PathValue("id"); id != "" {
		input.UeId = id
	}
	if input.UeId == "" {
		input.UeId = uuid.NewString()
	}
	if input.RequestId == "" {
		input.RequestId = telemetry.RequestID(r.Context())
	}
	if input.GnbId == "" {
		input.GnbId = "gnb-lab-01"
	}
	result, err := a.GNB.RegisterUE(r.Context(), input)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, map[string]any{"ue_id": input.UeId, "registration": result})
}
func (a *API) packet(w http.ResponseWriter, r *http.Request) {
	input := new(corev1.Packet)
	if err := decode(w, r, input); err != nil {
		fail(w, err)
		return
	}
	if input.RequestId == "" {
		input.RequestId = telemetry.RequestID(r.Context())
	}
	if input.Timestamp == "" {
		input.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if input.Destination == "" {
		input.Destination = "203.0.113.10"
	}
	if input.Source == "" {
		session, err := a.SMF.GetSession(r.Context(), &corev1.SessionRequest{SessionId: input.SessionId})
		if err != nil {
			fail(w, err)
			return
		}
		input.Source = session.IpAddress
	}
	result, err := a.GNB.SendPacket(r.Context(), input)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, result)
}
func (a *API) Statuses(ctx context.Context) []ServiceStatus {
	items := make([]ServiceStatus, 0, len(a.Connections))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, connection := range a.Connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()
			result, err := healthv1.NewHealthClient(connection).Check(checkCtx, &healthv1.HealthCheckRequest{})
			state := "UP"
			if err != nil || result.GetStatus() != healthv1.HealthCheckResponse_SERVING {
				state = "DOWN"
			}
			mu.Lock()
			items = append(items, ServiceStatus{name, state, float64(time.Since(started).Microseconds()) / 1000})
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ues, ueErr := a.AMF.ListUEs(ctx, &corev1.Empty{})
	sessions, sessionErr := a.SMF.ListSessions(ctx, &corev1.SessionFilter{})
	traffic, trafficErr := a.UPF.GetStats(ctx, &corev1.StatsRequest{})
	registered, active := 0, 0
	ueItems := []*corev1.UE{}
	sessionItems := []*corev1.Session{}
	warnings := []string{}
	if ueErr == nil {
		ueItems = ues.Equipments
		for _, ue := range ueItems {
			if ue.State == "REGISTERED" || ue.State == "SESSION_ACTIVE" {
				registered++
			}
		}
	} else {
		warnings = append(warnings, "AMF: "+status.Code(ueErr).String())
	}
	if sessionErr == nil {
		sessionItems = sessions.Sessions
		for _, session := range sessionItems {
			if session.State == "ACTIVE" {
				active++
			}
		}
	} else {
		warnings = append(warnings, "SMF: "+status.Code(sessionErr).String())
	}
	if trafficErr != nil {
		warnings = append(warnings, "UPF: "+status.Code(trafficErr).String())
		traffic = &corev1.Counters{}
	}
	write(w, 200, map[string]any{"registered_ues": registered, "active_sessions": active, "traffic": traffic, "services": a.Statuses(ctx), "ues": ueItems, "sessions": sessionItems, "warnings": warnings, "timestamp": time.Now().UTC(), "links": map[string]string{"grafana": config.Env("GRAFANA_URL", "http://127.0.0.1:3005"), "prometheus": config.Env("PROMETHEUS_URL", "http://127.0.0.1:9095"), "jaeger": config.Env("JAEGER_URL", "http://127.0.0.1:16686")}})
}
func (a *API) startJob(w http.ResponseWriter, r *http.Request) {
	input := struct {
		Count       int  `json:"count"`
		Concurrency int  `json:"concurrency"`
		Full        bool `json:"full"`
	}{Count: 10, Concurrency: 10}
	if err := decode(w, r, &input); err != nil {
		fail(w, err)
		return
	}
	if input.Count < 1 || input.Count > 1000 || input.Concurrency < 1 || input.Concurrency > 100 {
		fail(w, status.Error(codes.InvalidArgument, "count 1..1000 and concurrency 1..100 required"))
		return
	}
	a.mu.Lock()
	for _, job := range a.jobs {
		if job.State == "RUNNING" {
			a.mu.Unlock()
			fail(w, status.Error(codes.FailedPrecondition, "a simulation is already running"))
			return
		}
	}
	if len(a.jobs) >= 20 {
		for id := range a.jobs {
			delete(a.jobs, id)
			break
		}
	}
	job := &Job{ID: uuid.NewString(), State: "RUNNING", Count: input.Count, Errors: []string{}}
	a.jobs[job.ID] = job
	initial := *job
	a.mu.Unlock()
	go a.runJob(job, input.Concurrency, input.Full)
	write(w, 202, initial)
}
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index]
}
func (a *API) runJob(job *Job, concurrency int, full bool) {
	started := time.Now()
	queue := make(chan int, job.Count)
	for i := 1; i <= job.Count; i++ {
		queue <- i
	}
	close(queue)
	var wg sync.WaitGroup
	durations := []float64{}
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				begin := time.Now()
				ctx, cancel := context.WithTimeout(a.root, a.Timeout)
				ueID := uuid.NewString()
				registration, err := a.GNB.RegisterUE(ctx, &corev1.RegisterUERequest{RequestId: uuid.NewString(), UeId: ueID, Supi: fmt.Sprintf("imsi-00101%010d", i), GnbId: "gnb-load-01", Credential: "demo-key"})
				if err == nil && (registration == nil || !registration.Accepted) {
					err = fmt.Errorf("registration refused")
				}
				if err == nil && full {
					session, createErr := a.GNB.CreateSession(ctx, &corev1.CreateSessionRequest{RequestId: uuid.NewString(), UeId: ueID})
					err = createErr
					if err == nil {
						_, err = a.GNB.SendPacket(ctx, &corev1.Packet{SessionId: session.SessionId, RequestId: uuid.NewString(), Source: session.IpAddress, Destination: "203.0.113.10", Size: 512})
					}
				}
				cancel()
				duration := float64(time.Since(begin).Microseconds()) / 1000
				a.mu.Lock()
				job.Completed++
				durations = append(durations, duration)
				if err != nil {
					job.Failed++
					if len(job.Errors) < 10 {
						job.Errors = append(job.Errors, err.Error())
					}
				} else {
					job.Succeeded++
				}
				a.mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Float64s(durations)
	a.mu.Lock()
	job.State = "COMPLETED"
	job.P50 = percentile(durations, .5)
	job.P95 = percentile(durations, .95)
	job.P99 = percentile(durations, .99)
	job.Duration = float64(time.Since(started).Microseconds()) / 1000
	a.mu.Unlock()
}
