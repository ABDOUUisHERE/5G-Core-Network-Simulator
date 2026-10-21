package session

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/domain"
	"core5g/internal/store"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"sync"
	"time"
)

type MobilityClient interface {
	GetUEState(context.Context, *corev1.UERequest, ...grpc.CallOption) (*corev1.UE, error)
	UpdateSession(context.Context, *corev1.SessionEvent, ...grpc.CallOption) (*corev1.Empty, error)
}
type RouteClient interface {
	InstallRoute(context.Context, *corev1.Route, ...grpc.CallOption) (*corev1.Route, error)
	RemoveRoute(context.Context, *corev1.SessionRequest, ...grpc.CallOption) (*corev1.Empty, error)
}
type Service struct {
	corev1.UnimplementedSMFServiceServer
	Store   *store.Store
	AMF     MobilityClient
	UPF     RouteClient
	Timeout time.Duration
	mu      sync.Mutex
}

func (s *Service) save(ctx context.Context, session *corev1.Session) error {
	return s.Store.Run(ctx, func(tx *store.Tx) error { return tx.Put("sessions", session.SessionId, session) })
}
func (s *Service) ListSessions(ctx context.Context, r *corev1.SessionFilter) (*corev1.SessionList, error) {
	if r.UeId != "" {
		if err := domain.IDs(r.UeId); err != nil {
			return nil, err
		}
	}
	var items []*corev1.Session
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var err error
		all, err := store.All[corev1.Session](tx, "sessions")
		if err != nil {
			return err
		}
		items = make([]*corev1.Session, 0)
		for _, item := range all {
			if r.UeId == "" || item.UeId == r.UeId {
				items = append(items, item)
			}
		}
		return nil
	})
	return &corev1.SessionList{Sessions: items}, domain.RPCError(err)
}
func (s *Service) GetSession(ctx context.Context, r *corev1.SessionRequest) (*corev1.Session, error) {
	if err := domain.IDs(r.SessionId); err != nil {
		return nil, err
	}
	item := new(corev1.Session)
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		found, err := tx.Get("sessions", r.SessionId, item)
		if err != nil {
			return err
		}
		if !found {
			return status.Error(codes.NotFound, "session not found")
		}
		return nil
	})
	return item, domain.RPCError(err)
}
func (s *Service) registered(ctx context.Context, id string) error {
	rpcCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	ue, err := s.AMF.GetUEState(rpcCtx, &corev1.UERequest{UeId: id})
	if err != nil {
		return domain.RPCError(err)
	}
	if ue == nil || (ue.State != "REGISTERED" && ue.State != "SESSION_ACTIVE") {
		return status.Error(codes.FailedPrecondition, "UE must be registered")
	}
	return nil
}
func (s *Service) CreateSession(ctx context.Context, r *corev1.CreateSessionRequest) (*corev1.Session, error) {
	if err := domain.IDs(r.RequestId, r.UeId); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.ListSessions(ctx, &corev1.SessionFilter{})
	if err != nil {
		return nil, err
	}
	for _, item := range list.Sessions {
		if item.RequestId == r.RequestId {
			if item.UeId != r.UeId {
				return nil, status.Error(codes.AlreadyExists, "request_id belongs to another UE")
			}
			return item, nil
		}
	}
	if err = s.registered(ctx, r.UeId); err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, item := range list.Sessions {
		if item.State == "ACTIVE" || item.State == "CREATING" || item.State == "RELEASING" {
			used[item.IpAddress] = true
		}
	}
	ip := ""
	for n := 2; n < 65535; n++ {
		candidate := fmt.Sprintf("10.45.%d.%d", n/256, n%256)
		if !used[candidate] {
			ip = candidate
			break
		}
	}
	if ip == "" {
		return nil, status.Error(codes.ResourceExhausted, "virtual address pool exhausted")
	}
	item := &corev1.Session{SessionId: uuid.NewString(), RequestId: r.RequestId, UeId: r.UeId, IpAddress: ip, State: "CREATING", CreatedAt: domain.Now()}
	if err = s.save(ctx, item); err != nil {
		return nil, domain.RPCError(err)
	}
	rpcCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	route, installErr := s.UPF.InstallRoute(rpcCtx, &corev1.Route{SessionId: item.SessionId, IpAddress: ip})
	cancel()
	if installErr == nil && (route == nil || !route.Active) {
		installErr = status.Error(codes.Unavailable, "UPF did not confirm route")
	}
	if installErr == nil {
		installErr = s.registered(ctx, r.UeId)
	}
	if installErr == nil {
		item.State = "ACTIVE"
		installErr = s.save(ctx, item)
	}
	if installErr != nil {
		item.State = "RELEASING"
		item.LastError = "CREATE_FAILED: " + status.Code(domain.RPCError(installErr)).String()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), s.Timeout*2)
		defer cleanupCancel()
		// Persist the cleanup intent before removing an uncertain external effect.
		if err = s.save(cleanupCtx, item); err != nil {
			return nil, domain.RPCError(err)
		}
		_, cleanupErr := s.release(cleanupCtx, item)
		if cleanupErr != nil {
			return nil, status.Error(codes.Unavailable, "session creation failed; IP reserved until route cleanup is confirmed")
		}
		return nil, domain.RPCError(installErr)
	}
	s.notify(ctx, item)
	return item, nil
}
func (s *Service) notify(ctx context.Context, item *corev1.Session) {
	revision := uint64(1)
	if item.State != "ACTIVE" {
		revision = 2
	}
	rpcCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.Timeout)
	defer cancel()
	// The reconciler retries delivery; lifecycle success is determined by durable SMF state.
	_, _ = s.AMF.UpdateSession(rpcCtx, &corev1.SessionEvent{UeId: item.UeId, SessionId: item.SessionId, Active: item.State == "ACTIVE", Revision: revision})
}
func (s *Service) release(ctx context.Context, item *corev1.Session) (*corev1.Session, error) {
	if item.State == "RELEASED" || item.State == "FAILED" {
		return item, nil
	}
	item.State = "RELEASING"
	if err := s.save(ctx, item); err != nil {
		return nil, domain.RPCError(err)
	}
	rpcCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	_, err := s.UPF.RemoveRoute(rpcCtx, &corev1.SessionRequest{SessionId: item.SessionId, RequestId: uuid.NewString()})
	cancel()
	if err != nil {
		return nil, domain.RPCError(err)
	}
	item.State = "RELEASED"
	item.ReleasedAt = domain.Now()
	if strings.HasPrefix(item.LastError, "CREATE_FAILED:") {
		item.State = "FAILED"
		item.IpAddress = ""
	} else {
		item.LastError = ""
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), s.Timeout)
	defer cleanupCancel()
	if err = s.save(cleanupCtx, item); err != nil {
		return nil, domain.RPCError(err)
	}
	s.notify(cleanupCtx, item)
	return item, nil
}
func (s *Service) ReleaseSession(ctx context.Context, r *corev1.SessionRequest) (*corev1.Session, error) {
	if err := domain.IDs(r.RequestId, r.SessionId); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.GetSession(ctx, r)
	if err != nil {
		return nil, err
	}
	return s.release(ctx, item)
}
func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.ListSessions(ctx, &corev1.SessionFilter{})
	if err != nil {
		return err
	}
	var lastErr error
	for _, item := range list.Sessions {
		if item.State == "CREATING" {
			item.LastError = "CREATE_FAILED: interrupted workflow"
			item.State = "RELEASING"
			if err = s.save(ctx, item); err != nil {
				lastErr = err
				continue
			}
		}
		if item.State == "RELEASING" {
			if _, err = s.release(ctx, item); err != nil {
				lastErr = err
			}
			continue
		}
		s.notify(ctx, item)
	}
	return lastErr
}
