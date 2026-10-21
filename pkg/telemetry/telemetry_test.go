package telemetry

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func TestFaultControlsRemainReachable(t *testing.T) {
	m := New("test", time.Second)
	s := &FaultService{Meter: m}
	ctx := context.Background()
	if _, err := s.SetFault(ctx, &corev1.Fault{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	handler := func(context.Context, any) (any, error) { return &corev1.Empty{}, nil }
	if _, err := m.Unary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/core5g.core.v1.AMFService/RegisterUE"}, handler); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := m.Unary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/core5g.core.v1.FaultService/SetFault"}, handler); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFault(ctx, &corev1.Fault{DelayMs: 30001}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.SetFault(ctx, &corev1.Fault{DelayMs: 20}); err != nil {
		t.Fatal(err)
	}
	timeout, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if _, err := m.Unary(timeout, nil, &grpc.UnaryServerInfo{FullMethod: "/test/Work"}, handler); status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
}
