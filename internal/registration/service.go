package registration

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/domain"
	"core5g/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"time"
)

type AuthClient interface {
	Authenticate(context.Context, *corev1.AuthenticateRequest, ...grpc.CallOption) (*corev1.AuthenticateResponse, error)
}
type SessionClient interface {
	ListSessions(context.Context, *corev1.SessionFilter, ...grpc.CallOption) (*corev1.SessionList, error)
	ReleaseSession(context.Context, *corev1.SessionRequest, ...grpc.CallOption) (*corev1.Session, error)
	CreateSession(context.Context, *corev1.CreateSessionRequest, ...grpc.CallOption) (*corev1.Session, error)
}
type registrationRecord struct {
	UEID     string                     `json:"ue_id"`
	SUPI     string                     `json:"supi"`
	Response *corev1.RegisterUEResponse `json:"response"`
}
type Service struct {
	corev1.UnimplementedAMFServiceServer
	Store   *store.Store
	AUSF    AuthClient
	SMF     SessionClient
	Timeout time.Duration
	mu      sync.Mutex
}

func (s *Service) RegisterUE(ctx context.Context, r *corev1.RegisterUERequest) (*corev1.RegisterUEResponse, error) {
	if err := domain.IDs(r.RequestId, r.UeId); err != nil {
		return nil, err
	}
	if !domain.SUPI(r.Supi) || r.GnbId == "" || len(r.GnbId) > 64 || len(r.Credential) > 80 {
		return nil, status.Error(codes.InvalidArgument, "invalid SUPI, gNB or demo credential")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var previous registrationRecord
	ue := &corev1.UE{UeId: r.UeId, Supi: r.Supi, GnbId: r.GnbId, State: "REGISTERING", UpdatedAt: domain.Now()}
	var immediate *corev1.RegisterUEResponse
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		found, err := tx.Get("registrations", r.RequestId, &previous)
		if err != nil {
			return err
		}
		if found {
			if previous.UEID != r.UeId || previous.SUPI != r.Supi {
				return status.Error(codes.AlreadyExists, "request_id belongs to another registration")
			}
			immediate = previous.Response
			return nil
		}
		var existing corev1.UE
		found, err = tx.Get("equipments", r.UeId, &existing)
		if err != nil {
			return err
		}
		if found && existing.Supi != r.Supi {
			return status.Error(codes.AlreadyExists, "UE identity belongs to another SUPI")
		}
		if found && (existing.State == "REGISTERED" || existing.State == "SESSION_ACTIVE") {
			immediate = &corev1.RegisterUEResponse{Accepted: true, RegistrationId: existing.RegistrationId, State: existing.State, ReasonCode: "AUTHORIZED"}
			return tx.Put("registrations", r.RequestId, registrationRecord{r.UeId, r.Supi, immediate})
		}
		return tx.Put("equipments", r.UeId, ue)
	})
	if err != nil {
		return nil, domain.RPCError(err)
	}
	if immediate != nil {
		return immediate, nil
	}
	authCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	auth, authErr := s.AUSF.Authenticate(authCtx, &corev1.AuthenticateRequest{Supi: r.Supi, Credential: r.Credential})
	cancel()
	response := &corev1.RegisterUEResponse{State: "DISCONNECTED", ReasonCode: "UNAVAILABLE"}
	if authErr == nil && auth != nil {
		response.ReasonCode = auth.ReasonCode
		if auth.ReasonCode == "AUTHORIZED" {
			response.Accepted = true
			response.State = "REGISTERED"
			response.RegistrationId = uuid.NewString()
		}
	}
	if authErr == nil && auth == nil {
		authErr = status.Error(codes.Unavailable, "authentication response absent")
	}
	if ctx.Err() != nil {
		authErr = ctx.Err()
		response.Accepted = false
		response.State = "DISCONNECTED"
		response.RegistrationId = ""
		response.ReasonCode = "UNAVAILABLE"
	}
	ue.State = response.State
	ue.RegistrationId = response.RegistrationId
	ue.UpdatedAt = domain.Now()
	if !response.Accepted {
		ue.LastError = response.ReasonCode
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), s.Timeout)
	defer cleanupCancel()
	err = s.Store.Run(cleanupCtx, func(tx *store.Tx) error {
		if err := tx.Put("equipments", ue.UeId, ue); err != nil {
			return err
		}
		// Transient dependency failures are retryable with the same request_id.
		if authErr != nil {
			return nil
		}
		return tx.Put("registrations", r.RequestId, registrationRecord{r.UeId, r.Supi, response})
	})
	if err != nil {
		return nil, domain.RPCError(err)
	}
	if authErr != nil {
		return nil, domain.RPCError(authErr)
	}
	return response, nil
}

func (s *Service) GetUEState(ctx context.Context, r *corev1.UERequest) (*corev1.UE, error) {
	if err := domain.IDs(r.UeId); err != nil {
		return nil, err
	}
	ue := new(corev1.UE)
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		found, err := tx.Get("equipments", r.UeId, ue)
		if err != nil {
			return err
		}
		if !found {
			return status.Error(codes.NotFound, "UE not found")
		}
		return nil
	})
	return ue, domain.RPCError(err)
}
func (s *Service) ListUEs(ctx context.Context, _ *corev1.Empty) (*corev1.UEList, error) {
	var items []*corev1.UE
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var err error
		items, err = store.All[corev1.UE](tx, "equipments")
		return err
	})
	return &corev1.UEList{Equipments: items}, domain.RPCError(err)
}
func (s *Service) CreateSession(ctx context.Context, r *corev1.CreateSessionRequest) (*corev1.Session, error) {
	if err := domain.IDs(r.RequestId, r.UeId); err != nil {
		return nil, err
	}
	if s.SMF == nil {
		return nil, status.Error(codes.Unavailable, "SMF unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	return s.SMF.CreateSession(ctx, r)
}
func (s *Service) DeregisterUE(ctx context.Context, r *corev1.UERequest) (*corev1.UE, error) {
	if err := domain.IDs(r.RequestId, r.UeId); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ue, err := s.GetUEState(ctx, r)
	if err != nil {
		return nil, err
	}
	// Close the registration gate before listing sessions. A racing SMF rechecks it.
	ue.State = "DISCONNECTED"
	ue.UpdatedAt = domain.Now()
	ue.LastError = ""
	if err = s.Store.Run(ctx, func(tx *store.Tx) error { return tx.Put("equipments", ue.UeId, ue) }); err != nil {
		return nil, domain.RPCError(err)
	}
	if s.SMF != nil {
		rpcCtx, cancel := context.WithTimeout(ctx, s.Timeout)
		list, listErr := s.SMF.ListSessions(rpcCtx, &corev1.SessionFilter{UeId: r.UeId})
		cancel()
		if listErr == nil && list != nil {
			for _, session := range list.Sessions {
				if session.State != "RELEASED" && session.State != "FAILED" {
					rpcCtx, cancel = context.WithTimeout(ctx, s.Timeout)
					_, listErr = s.SMF.ReleaseSession(rpcCtx, &corev1.SessionRequest{RequestId: uuid.NewString(), SessionId: session.SessionId})
					cancel()
					if listErr != nil {
						break
					}
				}
			}
		}
		if listErr != nil {
			ue.LastError = "CLEANUP_PENDING"
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), s.Timeout)
			defer cleanupCancel()
			if saveErr := s.Store.Run(cleanupCtx, func(tx *store.Tx) error { return tx.Put("equipments", ue.UeId, ue) }); saveErr != nil {
				return nil, domain.RPCError(saveErr)
			}
			return nil, domain.RPCError(listErr)
		}
	}
	return ue, nil
}
func (s *Service) UpdateSession(ctx context.Context, r *corev1.SessionEvent) (*corev1.Empty, error) {
	if err := domain.IDs(r.UeId, r.SessionId); err != nil {
		return nil, err
	}
	if r.Revision == 0 {
		return nil, status.Error(codes.InvalidArgument, "event revision required")
	}
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var old corev1.SessionEvent
		found, err := tx.Get("session_events", r.SessionId, &old)
		if err != nil {
			return err
		}
		if found && old.Revision >= r.Revision {
			return nil
		}
		var ue corev1.UE
		found, err = tx.Get("equipments", r.UeId, &ue)
		if err != nil {
			return err
		}
		if !found {
			return status.Error(codes.NotFound, "UE not found")
		}
		if err = tx.Put("session_events", r.SessionId, r); err != nil {
			return err
		}
		if ue.State == "REGISTERED" || ue.State == "SESSION_ACTIVE" {
			events, err := store.All[corev1.SessionEvent](tx, "session_events")
			if err != nil {
				return err
			}
			ue.State = "REGISTERED"
			for _, event := range events {
				if event.UeId == r.UeId && event.Active {
					ue.State = "SESSION_ACTIVE"
					break
				}
			}
			ue.UpdatedAt = domain.Now()
			return tx.Put("equipments", ue.UeId, &ue)
		}
		return nil
	})
	return &corev1.Empty{}, domain.RPCError(err)
}
func (s *Service) Recover(ctx context.Context) error {
	return s.Store.Run(ctx, func(tx *store.Tx) error {
		items, err := store.All[corev1.UE](tx, "equipments")
		if err != nil {
			return err
		}
		for _, ue := range items {
			if ue.State == "REGISTERING" {
				ue.State = "DISCONNECTED"
				ue.LastError = "INTERRUPTED_REGISTRATION"
				ue.UpdatedAt = domain.Now()
				if err := tx.Put("equipments", ue.UeId, ue); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
