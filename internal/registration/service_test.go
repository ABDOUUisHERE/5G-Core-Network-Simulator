package registration

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

type fakeAuth struct {
	reason string
	err    error
	wait   bool
}

func (f fakeAuth) Authenticate(ctx context.Context, _ *corev1.AuthenticateRequest, _ ...grpc.CallOption) (*corev1.AuthenticateResponse, error) {
	if f.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.reason == "nil" {
		return nil, nil
	}
	return &corev1.AuthenticateResponse{ReasonCode: f.reason}, f.err
}

type fakeSMF struct {
	sessions []*corev1.Session
	err      error
	released int
}

func (f *fakeSMF) ListSessions(context.Context, *corev1.SessionFilter, ...grpc.CallOption) (*corev1.SessionList, error) {
	return &corev1.SessionList{Sessions: f.sessions}, f.err
}
func (f *fakeSMF) ReleaseSession(context.Context, *corev1.SessionRequest, ...grpc.CallOption) (*corev1.Session, error) {
	f.released++
	return &corev1.Session{State: "RELEASED"}, f.err
}
func (f *fakeSMF) CreateSession(context.Context, *corev1.CreateSessionRequest, ...grpc.CallOption) (*corev1.Session, error) {
	return &corev1.Session{State: "ACTIVE"}, f.err
}
func request() *corev1.RegisterUERequest {
	return &corev1.RegisterUERequest{RequestId: uuid.NewString(), UeId: uuid.NewString(), Supi: "imsi-001010000000001", GnbId: "gnb-test", Credential: "demo-key"}
}
func service(auth fakeAuth) *Service {
	return &Service{Store: store.NewMemory("amf"), AUSF: auth, Timeout: 20 * time.Millisecond}
}

func TestRegistrationIdempotencyAndConcurrency(t *testing.T) {
	s := service(fakeAuth{reason: "AUTHORIZED"})
	r := request()
	var wg sync.WaitGroup
	for range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := s.RegisterUE(context.Background(), r)
			if err != nil || !out.Accepted {
				t.Errorf("registration: %v %v", out, err)
			}
		}()
	}
	wg.Wait()
	list, err := s.ListUEs(context.Background(), &corev1.Empty{})
	if err != nil || len(list.Equipments) != 1 {
		t.Fatal(list, err)
	}
	first, err := s.RegisterUE(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.RequestId = uuid.NewString()
	second, err := s.RegisterUE(context.Background(), r)
	if err != nil || first.RegistrationId != second.RegistrationId {
		t.Fatal(first, second, err)
	}
	r.UeId = uuid.NewString()
	if _, err = s.RegisterUE(context.Background(), r); status.Code(err) != codes.AlreadyExists {
		t.Fatal(err)
	}
	r = request()
	r.UeId = list.Equipments[0].UeId
	r.Supi = "imsi-001010000000002"
	if _, err = s.RegisterUE(context.Background(), r); status.Code(err) != codes.AlreadyExists {
		t.Fatal(err)
	}
}
func TestRegistrationRejectionAndTimeout(t *testing.T) {
	for _, reason := range []string{"UNKNOWN_SUBSCRIBER", "DISABLED_SUBSCRIBER", "INVALID_CREDENTIALS"} {
		t.Run(reason, func(t *testing.T) {
			s := service(fakeAuth{reason: reason})
			r := request()
			out, err := s.RegisterUE(context.Background(), r)
			if err != nil || out.Accepted || out.State != "DISCONNECTED" {
				t.Fatal(out, err)
			}
		})
	}
	for _, auth := range []fakeAuth{{wait: true}, {reason: "nil"}, {err: status.Error(codes.Unavailable, "offline")}} {
		s := service(auth)
		r := request()
		if _, err := s.RegisterUE(context.Background(), r); err == nil {
			t.Fatal("dependency failure accepted")
		}
		ue, err := s.GetUEState(context.Background(), &corev1.UERequest{UeId: r.UeId})
		if err != nil || ue.State != "DISCONNECTED" {
			t.Fatal(ue, err)
		}
	}
	s := service(fakeAuth{reason: "AUTHORIZED"})
	r := request()
	r.RequestId = "invalid"
	if _, err := s.RegisterUE(context.Background(), r); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	r = request()
	r.GnbId = ""
	if _, err := s.RegisterUE(context.Background(), r); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := s.GetUEState(context.Background(), &corev1.UERequest{UeId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
func TestSessionEventsDeregistrationAndRecovery(t *testing.T) {
	ctx := context.Background()
	s := service(fakeAuth{reason: "AUTHORIZED"})
	r := request()
	if _, err := s.RegisterUE(ctx, r); err != nil {
		t.Fatal(err)
	}
	event := &corev1.SessionEvent{UeId: r.UeId, SessionId: uuid.NewString(), Active: true, Revision: 1}
	if _, err := s.UpdateSession(ctx, event); err != nil {
		t.Fatal(err)
	}
	ue, _ := s.GetUEState(ctx, &corev1.UERequest{UeId: r.UeId})
	if ue.State != "SESSION_ACTIVE" {
		t.Fatal(ue)
	}
	event.Active = false
	event.Revision = 2
	if _, err := s.UpdateSession(ctx, event); err != nil {
		t.Fatal(err)
	}
	event.Active = true
	event.Revision = 1
	if _, err := s.UpdateSession(ctx, event); err != nil {
		t.Fatal(err)
	}
	ue, _ = s.GetUEState(ctx, &corev1.UERequest{UeId: r.UeId})
	if ue.State != "REGISTERED" {
		t.Fatal("late event resurrected session", ue)
	}
	smf := &fakeSMF{sessions: []*corev1.Session{{SessionId: event.SessionId, State: "ACTIVE"}}}
	s.SMF = smf
	if _, err := s.CreateSession(ctx, &corev1.CreateSessionRequest{RequestId: uuid.NewString(), UeId: r.UeId}); err != nil {
		t.Fatal(err)
	}
	ue, err := s.DeregisterUE(ctx, &corev1.UERequest{RequestId: uuid.NewString(), UeId: r.UeId})
	if err != nil || ue.State != "DISCONNECTED" || smf.released != 1 {
		t.Fatal(ue, err, smf.released)
	}
	smf.err = status.Error(codes.Unavailable, "offline")
	if _, err = s.DeregisterUE(ctx, &corev1.UERequest{RequestId: uuid.NewString(), UeId: r.UeId}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if err = s.Store.Run(ctx, func(tx *store.Tx) error { ue.State = "REGISTERING"; return tx.Put("equipments", ue.UeId, ue) }); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	ue, _ = s.GetUEState(ctx, &corev1.UERequest{UeId: r.UeId})
	if ue.State != "DISCONNECTED" || ue.LastError != "INTERRUPTED_REGISTRATION" {
		t.Fatal(ue)
	}
}
