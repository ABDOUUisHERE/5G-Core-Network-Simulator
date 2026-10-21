package routing

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"testing"
	"time"
)

type sessionReader struct{ state string }

func (f sessionReader) GetSession(context.Context, *corev1.SessionRequest, ...grpc.CallOption) (*corev1.Session, error) {
	return &corev1.Session{State: f.state}, nil
}
func setup(t *testing.T) (*Service, *corev1.Packet) {
	t.Helper()
	s := &Service{Store: store.NewMemory("upf"), SMF: sessionReader{"ACTIVE"}, Timeout: time.Second}
	id := uuid.NewString()
	if _, err := s.InstallRoute(context.Background(), &corev1.Route{SessionId: id, IpAddress: "10.45.0.2"}); err != nil {
		t.Fatal(err)
	}
	return s, &corev1.Packet{SessionId: id, RequestId: uuid.NewString(), Source: "10.45.0.2", Destination: "203.0.113.10", Size: 512}
}
func TestSeededTrafficAndCounters(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	cfg := &corev1.TrafficConfig{LatencyMs: 1, JitterMs: 1, LossPercent: 40, Seed: 42}
	if _, err := s.ConfigureTraffic(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	a, err := s.SendPacket(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SendPacket(ctx, p)
	if err != nil || a.Lost != b.Lost || a.LatencyMs != b.LatencyMs {
		t.Fatal("seed not reproducible", a, b, err)
	}
	cfg.LossPercent = 100
	if _, err = s.ConfigureTraffic(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	lost, err := s.SendPacket(ctx, p)
	if err != nil || !lost.Lost {
		t.Fatal(lost, err)
	}
	cfg.LossPercent = 0
	if _, err = s.ConfigureTraffic(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	received, err := s.SendPacket(ctx, p)
	if err != nil || !received.Received {
		t.Fatal(received, err)
	}
	stats, err := s.GetStats(ctx, &corev1.StatsRequest{})
	if err != nil || stats.PacketsSent != 4 || stats.BytesSent != 2048 || stats.PacketsReceived+stats.PacketsLost != 4 {
		t.Fatal(stats, err)
	}
	if _, err = s.GetStats(ctx, &corev1.StatsRequest{SessionId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
func TestInactiveSessionAndRouteProtection(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.SMF = sessionReader{"RELEASED"}
	if _, err := s.SendPacket(ctx, p); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	s.SMF = sessionReader{"ACTIVE"}
	if _, err := s.InstallRoute(ctx, &corev1.Route{SessionId: p.SessionId, IpAddress: p.Source}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallRoute(ctx, &corev1.Route{SessionId: uuid.NewString(), IpAddress: p.Source}); status.Code(err) != codes.AlreadyExists {
		t.Fatal(err)
	}
	p.Source = "10.45.0.99"
	if _, err := s.SendPacket(ctx, p); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := s.RemoveRoute(ctx, &corev1.SessionRequest{SessionId: p.SessionId}); err != nil {
			t.Fatal(err)
		}
	}
	p.Source = "10.45.0.2"
	if _, err := s.SendPacket(ctx, p); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	stats, _ := s.GetStats(ctx, &corev1.StatsRequest{SessionId: p.SessionId})
	if stats.PacketsRejected != 3 || stats.BytesRejected != 1536 {
		t.Fatal(stats)
	}
	if _, err := s.InstallRoute(ctx, &corev1.Route{SessionId: p.SessionId, IpAddress: p.Source}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}
func TestInvalidTrafficAndCanceledPacket(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	for _, cfg := range []*corev1.TrafficConfig{{LossPercent: 101}, {LatencyMs: -1}, {JitterMs: math.NaN()}, {LatencyMs: 2000}} {
		if _, err := s.ConfigureTraffic(ctx, cfg); status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
	}
	if _, err := s.ConfigureTraffic(ctx, &corev1.TrafficConfig{LatencyMs: 1000}); err != nil {
		t.Fatal(err)
	}
	timeout, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if _, err := s.SendPacket(timeout, p); status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
	p.Size = 0
	if _, err := s.SendPacket(ctx, p); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	p.Size = 32
	p.Timestamp = "bad"
	if _, err := s.SendPacket(ctx, p); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
