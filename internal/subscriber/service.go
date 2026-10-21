package subscriber

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/domain"
	"core5g/internal/store"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	corev1.UnimplementedUDMServiceServer
	Store *store.Store
}

func (s *Service) GetSubscriber(ctx context.Context, r *corev1.SubscriberRequest) (*corev1.Subscriber, error) {
	if !domain.SUPI(r.Supi) {
		return nil, status.Error(codes.InvalidArgument, "synthetic SUPI must match imsi-00101 plus ten digits")
	}
	result := new(corev1.Subscriber)
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		ok, err := tx.Get("subscribers", r.Supi, result)
		if err != nil {
			return err
		}
		if !ok {
			return status.Error(codes.NotFound, "unknown subscriber")
		}
		return nil
	})
	return result, domain.RPCError(err)
}
func (s *Service) PutSubscriber(ctx context.Context, r *corev1.Subscriber) (*corev1.Subscriber, error) {
	if !domain.SUPI(r.Supi) || (r.Status != "ENABLED" && r.Status != "DISABLED") || r.AuthProfile == "" || len(r.AuthProfile) > 80 {
		return nil, status.Error(codes.InvalidArgument, "valid synthetic SUPI, status and demo auth_profile required")
	}
	err := s.Store.Run(ctx, func(tx *store.Tx) error { return tx.Put("subscribers", r.Supi, r) })
	return r, domain.RPCError(err)
}
func (s *Service) ListSubscribers(ctx context.Context, _ *corev1.Empty) (*corev1.SubscriberList, error) {
	var items []*corev1.Subscriber
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var err error
		items, err = store.All[corev1.Subscriber](tx, "subscribers")
		return err
	})
	return &corev1.SubscriberList{Subscribers: items}, domain.RPCError(err)
}
func (s *Service) Seed(ctx context.Context, count int) error {
	return s.Store.Run(ctx, func(tx *store.Tx) error {
		for i := 1; i <= count; i++ {
			supi := fmt.Sprintf("imsi-00101%010d", i)
			var existing corev1.Subscriber
			found, err := tx.Get("subscribers", supi, &existing)
			if err != nil {
				return err
			}
			if found {
				continue
			}
			if err := tx.Put("subscribers", supi, &corev1.Subscriber{Supi: supi, Status: "ENABLED", AuthProfile: "demo-key"}); err != nil {
				return err
			}
		}
		return nil
	})
}
