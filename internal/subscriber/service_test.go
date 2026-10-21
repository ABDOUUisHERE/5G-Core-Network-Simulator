package subscriber

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestProfilesAndSeedPreservation(t *testing.T) {
	s := &Service{Store: store.NewMemory("udm")}
	ctx := context.Background()
	if err := s.Seed(ctx, 3); err != nil {
		t.Fatal(err)
	}
	supi := "imsi-001010000000001"
	if _, err := s.PutSubscriber(ctx, &corev1.Subscriber{Supi: supi, Status: "DISABLED", AuthProfile: "demo-key"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(ctx, 3); err != nil {
		t.Fatal(err)
	}
	profile, err := s.GetSubscriber(ctx, &corev1.SubscriberRequest{Supi: supi})
	if err != nil || profile.Status != "DISABLED" {
		t.Fatalf("seed overwrote profile: %v %v", profile, err)
	}
	list, err := s.ListSubscribers(ctx, &corev1.Empty{})
	if err != nil || len(list.Subscribers) != 3 {
		t.Fatal(list, err)
	}
	if _, err = s.GetSubscriber(ctx, &corev1.SubscriberRequest{Supi: "imsi-001010000000999"}); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	if _, err = s.GetSubscriber(ctx, &corev1.SubscriberRequest{Supi: "real-identity"}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	if _, err = s.PutSubscriber(ctx, &corev1.Subscriber{Supi: supi, Status: "BOGUS"}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
