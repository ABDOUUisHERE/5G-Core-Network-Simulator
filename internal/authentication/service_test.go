package authentication

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type fakeUDM struct {
	profile *corev1.Subscriber
	err     error
}

func (f fakeUDM) GetSubscriber(context.Context, *corev1.SubscriberRequest, ...grpc.CallOption) (*corev1.Subscriber, error) {
	return f.profile, f.err
}
func TestAuthenticationReasons(t *testing.T) {
	for _, test := range []struct {
		name, status, credential, reason string
		err                              error
		code                             codes.Code
	}{
		{name: "known", status: "ENABLED", credential: "demo-key", reason: "AUTHORIZED"},
		{name: "disabled", status: "DISABLED", credential: "demo-key", reason: "DISABLED_SUBSCRIBER"},
		{name: "invalid credential", status: "ENABLED", credential: "incorrect", reason: "INVALID_CREDENTIALS"},
		{name: "unknown", err: status.Error(codes.NotFound, "missing"), reason: "UNKNOWN_SUBSCRIBER"},
		{name: "unavailable", err: status.Error(codes.Unavailable, "down"), code: codes.Unavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{UDM: fakeUDM{&corev1.Subscriber{Status: test.status, AuthProfile: "demo-key"}, test.err}, Timeout: time.Second}
			result, err := s.Authenticate(context.Background(), &corev1.AuthenticateRequest{Supi: "imsi-001010000000001", Credential: test.credential})
			if status.Code(err) != test.code {
				t.Fatal(err)
			}
			if err == nil && result.ReasonCode != test.reason {
				t.Fatal(result)
			}
		})
	}
	s := &Service{UDM: fakeUDM{}, Timeout: time.Second}
	if _, err := s.Authenticate(context.Background(), &corev1.AuthenticateRequest{Supi: "imsi-001010000000001"}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), &corev1.AuthenticateRequest{Supi: "bad"}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
