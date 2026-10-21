package session

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"testing"
	"time"
)

type fakeMobility struct {
	state  string
	events []*corev1.SessionEvent
	mu     sync.Mutex
}

func (f *fakeMobility) GetUEState(context.Context, *corev1.UERequest, ...grpc.CallOption) (*corev1.UE, error) {
	return &corev1.UE{State: f.state}, nil
}
func (f *fakeMobility) UpdateSession(_ context.Context, r *corev1.SessionEvent, _ ...grpc.CallOption) (*corev1.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, r)
	return &corev1.Empty{}, nil
}

type fakeRoutes struct {
	installErr, removeErr error
	installed             map[string]string
	removed               int
}

func (f *fakeRoutes) InstallRoute(_ context.Context, r *corev1.Route, _ ...grpc.CallOption) (*corev1.Route, error) {
	if f.installErr != nil {
		return nil, f.installErr
	}
	if f.installed == nil {
		f.installed = map[string]string{}
	}
	f.installed[r.SessionId] = r.IpAddress
	return &corev1.Route{SessionId: r.SessionId, IpAddress: r.IpAddress, Active: true}, nil
}
func (f *fakeRoutes) RemoveRoute(_ context.Context, r *corev1.SessionRequest, _ ...grpc.CallOption) (*corev1.Empty, error) {
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	delete(f.installed, r.SessionId)
	f.removed++
	return &corev1.Empty{}, nil
}
func newService() (*Service, *fakeMobility, *fakeRoutes) {
	amf := &fakeMobility{state: "REGISTERED"}
	upf := &fakeRoutes{}
	return &Service{Store: store.NewMemory("smf"), AMF: amf, UPF: upf, Timeout: time.Second}, amf, upf
}
func createRequest() *corev1.CreateSessionRequest {
	return &corev1.CreateSessionRequest{RequestId: uuid.NewString(), UeId: uuid.NewString()}
}
func TestSessionIdempotencyAndRelease(t *testing.T) {
	s, _, upf := newService()
	ctx := context.Background()
	r := createRequest()
	one, err := s.CreateSession(ctx, r)
	if err != nil || one.State != "ACTIVE" {
		t.Fatal(one, err)
	}
	duplicate, err := s.CreateSession(ctx, r)
	if err != nil || duplicate.SessionId != one.SessionId || len(upf.installed) != 1 {
		t.Fatal(duplicate, err)
	}
	r.UeId = uuid.NewString()
	if _, err = s.CreateSession(ctx, r); status.Code(err) != codes.AlreadyExists {
		t.Fatal(err)
	}
	for range 2 {
		released, err := s.ReleaseSession(ctx, &corev1.SessionRequest{RequestId: uuid.NewString(), SessionId: one.SessionId})
		if err != nil || released.State != "RELEASED" {
			t.Fatal(released, err)
		}
	}
	if upf.removed != 1 {
		t.Fatal("duplicate cleanup", upf.removed)
	}
	next, err := s.CreateSession(ctx, createRequest())
	if err != nil || next.IpAddress != one.IpAddress {
		t.Fatal("cleaned IP not reusable", next, err)
	}
	if _, err = s.GetSession(ctx, &corev1.SessionRequest{SessionId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
func TestSessionPreconditionsAndCompensation(t *testing.T) {
	ctx := context.Background()
	s, amf, upf := newService()
	amf.state = "DISCONNECTED"
	if _, err := s.CreateSession(ctx, createRequest()); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	amf.state = "REGISTERED"
	upf.installErr = status.Error(codes.Unavailable, "UPF offline")
	r := createRequest()
	if _, err := s.CreateSession(ctx, r); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	list, _ := s.ListSessions(ctx, &corev1.SessionFilter{UeId: r.UeId})
	if len(list.Sessions) != 1 || list.Sessions[0].State != "FAILED" || list.Sessions[0].IpAddress != "" {
		t.Fatal(list)
	}
	upf.removeErr = status.Error(codes.Unavailable, "cannot confirm cleanup")
	r = createRequest()
	if _, err := s.CreateSession(ctx, r); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	list, _ = s.ListSessions(ctx, &corev1.SessionFilter{UeId: r.UeId})
	if list.Sessions[0].State != "RELEASING" || list.Sessions[0].IpAddress == "" {
		t.Fatal("unsafe IP reuse", list)
	}
	upf.removeErr = nil
	upf.installErr = nil
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListSessions(ctx, &corev1.SessionFilter{UeId: r.UeId})
	if list.Sessions[0].State != "FAILED" || list.Sessions[0].IpAddress != "" {
		t.Fatal(list)
	}
	if _, err := s.CreateSession(ctx, &corev1.CreateSessionRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.ReleaseSession(ctx, &corev1.SessionRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
func TestConcurrentSessionIPsAndInterruptedRecovery(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newService()
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.CreateSession(ctx, createRequest()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	list, err := s.ListSessions(ctx, &corev1.SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ips := map[string]bool{}
	for _, item := range list.Sessions {
		if ips[item.IpAddress] {
			t.Fatal("duplicate IP", item.IpAddress)
		}
		ips[item.IpAddress] = true
	}
	interrupted := list.Sessions[0]
	interrupted.State = "CREATING"
	if err = s.save(ctx, interrupted); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.GetSession(ctx, &corev1.SessionRequest{SessionId: interrupted.SessionId})
	if err != nil || recovered.State != "FAILED" {
		t.Fatal(recovered, err)
	}
}
