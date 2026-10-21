package authentication

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/domain"
	"crypto/subtle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type SubscriberClient interface {
	GetSubscriber(context.Context, *corev1.SubscriberRequest, ...grpc.CallOption) (*corev1.Subscriber, error)
}
type Service struct {
	corev1.UnimplementedAUSFServiceServer
	UDM     SubscriberClient
	Timeout time.Duration
}

func (s *Service) Authenticate(ctx context.Context, r *corev1.AuthenticateRequest) (*corev1.AuthenticateResponse, error) {
	if !domain.SUPI(r.Supi) || len(r.Credential) > 80 {
		return nil, status.Error(codes.InvalidArgument, "invalid synthetic authentication input")
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	profile, err := s.UDM.GetSubscriber(ctx, &corev1.SubscriberRequest{Supi: r.Supi})
	reason := "AUTHORIZED"
	switch {
	case status.Code(err) == codes.NotFound:
		reason = "UNKNOWN_SUBSCRIBER"
	case err != nil:
		return &corev1.AuthenticateResponse{ReasonCode: "UNAVAILABLE"}, domain.RPCError(err)
	case profile == nil:
		return nil, status.Error(codes.Unavailable, "subscriber response absent")
	case profile.Status != "ENABLED":
		reason = "DISABLED_SUBSCRIBER"
	case subtle.ConstantTimeCompare([]byte(profile.AuthProfile), []byte(r.Credential)) != 1:
		reason = "INVALID_CREDENTIALS"
	}
	return &corev1.AuthenticateResponse{ReasonCode: reason}, nil
}
