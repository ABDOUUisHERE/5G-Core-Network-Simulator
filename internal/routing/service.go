package routing

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/domain"
	"core5g/internal/store"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"net"
	"time"
)

type SessionReader interface {
	GetSession(context.Context, *corev1.SessionRequest, ...grpc.CallOption) (*corev1.Session, error)
}
type Service struct {
	corev1.UnimplementedUPFServiceServer
	Store   *store.Store
	SMF     SessionReader
	Timeout time.Duration
}

func (s *Service) InstallRoute(ctx context.Context, r *corev1.Route) (*corev1.Route, error) {
	if err := domain.IDs(r.SessionId); err != nil {
		return nil, err
	}
	if net.ParseIP(r.IpAddress) == nil {
		return nil, status.Error(codes.InvalidArgument, "valid virtual IP required")
	}
	var result *corev1.Route
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var old corev1.Route
		found, err := tx.Get("routes", r.SessionId, &old)
		if err != nil {
			return err
		}
		if found {
			if !old.Active || old.IpAddress != r.IpAddress {
				return status.Error(codes.FailedPrecondition, "route identity cannot be changed or reactivated")
			}
			result = &old
			return nil
		}
		routes, err := store.All[corev1.Route](tx, "routes")
		if err != nil {
			return err
		}
		for _, route := range routes {
			if route.Active && route.IpAddress == r.IpAddress {
				return status.Error(codes.AlreadyExists, "virtual IP already routed")
			}
		}
		result = &corev1.Route{SessionId: r.SessionId, IpAddress: r.IpAddress, Active: true, Counters: &corev1.Counters{}}
		return tx.Put("routes", r.SessionId, result)
	})
	return result, domain.RPCError(err)
}
func (s *Service) RemoveRoute(ctx context.Context, r *corev1.SessionRequest) (*corev1.Empty, error) {
	if err := domain.IDs(r.SessionId); err != nil {
		return nil, err
	}
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		var route corev1.Route
		found, err := tx.Get("routes", r.SessionId, &route)
		if err != nil || !found {
			return err
		}
		route.Active = false
		return tx.Put("routes", r.SessionId, &route)
	})
	return &corev1.Empty{}, domain.RPCError(err)
}
func (s *Service) ConfigureTraffic(ctx context.Context, r *corev1.TrafficConfig) (*corev1.TrafficConfig, error) {
	for _, v := range []float64{r.LatencyMs, r.JitterMs, r.LossPercent} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, status.Error(codes.InvalidArgument, "finite nonnegative traffic settings required")
		}
	}
	if r.LatencyMs > 1000 || r.JitterMs > 1000 || r.LossPercent > 100 {
		return nil, status.Error(codes.InvalidArgument, "latency/jitter <= 1000 ms and loss <= 100 required")
	}
	err := s.Store.Run(ctx, func(tx *store.Tx) error { return tx.Put("settings", "traffic", r) })
	return r, domain.RPCError(err)
}
func (s *Service) reject(ctx context.Context, r *corev1.Packet) error {
	return s.Store.Run(ctx, func(tx *store.Tx) error {
		var route corev1.Route
		found, err := tx.Get("routes", r.SessionId, &route)
		if err != nil || !found {
			return err
		}
		if route.Counters == nil {
			route.Counters = &corev1.Counters{}
		}
		route.Counters.PacketsRejected++
		route.Counters.BytesRejected += uint64(r.Size)
		return tx.Put("routes", r.SessionId, &route)
	})
}
func (s *Service) SendPacket(ctx context.Context, r *corev1.Packet) (*corev1.PacketResult, error) {
	if err := domain.IDs(r.SessionId, r.RequestId); err != nil {
		return nil, err
	}
	if r.Size == 0 || r.Size > 65507 || net.ParseIP(r.Source) == nil || net.ParseIP(r.Destination) == nil {
		return nil, status.Error(codes.InvalidArgument, "packet size must be 1..65507 bytes with valid virtual IPs")
	}
	if r.Timestamp != "" {
		if _, err := time.Parse(time.RFC3339Nano, r.Timestamp); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid packet timestamp")
		}
	}
	if s.SMF == nil {
		return nil, status.Error(codes.Unavailable, "SMF dependency unavailable")
	}
	if s.SMF != nil {
		rpcCtx, cancel := context.WithTimeout(ctx, s.Timeout)
		session, err := s.SMF.GetSession(rpcCtx, &corev1.SessionRequest{SessionId: r.SessionId})
		cancel()
		if err != nil {
			return nil, domain.RPCError(err)
		}
		if session == nil || session.State != "ACTIVE" {
			if err = s.reject(ctx, r); err != nil {
				return nil, domain.RPCError(err)
			}
			return nil, status.Error(codes.FailedPrecondition, "packet requires an ACTIVE session")
		}
	}
	settings := &corev1.TrafficConfig{LatencyMs: 2, JitterMs: 1, LossPercent: 1, Seed: 42}
	err := s.Store.Run(ctx, func(tx *store.Tx) error { _, err := tx.Get("settings", "traffic", settings); return err })
	if err != nil {
		return nil, domain.RPCError(err)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", settings.Seed, r.RequestId)))
	random := float64(binary.BigEndian.Uint64(digest[:8])) / float64(^uint64(0))
	latency := math.Max(0, settings.LatencyMs+(random*2-1)*settings.JitterMs)
	lossSample := float64(binary.BigEndian.Uint64(digest[8:16])) / float64(^uint64(0)) * 100
	lost := settings.LossPercent >= 100 || lossSample < settings.LossPercent
	timer := time.NewTimer(time.Duration(latency * float64(time.Millisecond)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, domain.RPCError(ctx.Err())
	case <-timer.C:
	}
	var counters *corev1.Counters
	var rejection bool
	err = s.Store.Run(ctx, func(tx *store.Tx) error {
		var route corev1.Route
		found, err := tx.Get("routes", r.SessionId, &route)
		if err != nil {
			return err
		}
		if !found {
			return status.Error(codes.FailedPrecondition, "route is absent")
		}
		if route.Counters == nil {
			route.Counters = &corev1.Counters{}
		}
		counters = route.Counters
		if !route.Active || route.IpAddress != r.Source {
			counters.PacketsRejected++
			counters.BytesRejected += uint64(r.Size)
			rejection = true
			return tx.Put("routes", r.SessionId, &route)
		}
		counters.PacketsSent++
		counters.BytesSent += uint64(r.Size)
		if lost {
			counters.PacketsLost++
			counters.BytesLost += uint64(r.Size)
		} else {
			counters.PacketsReceived++
			counters.BytesReceived += uint64(r.Size)
		}
		return tx.Put("routes", r.SessionId, &route)
	})
	if err != nil {
		return nil, domain.RPCError(err)
	}
	if rejection {
		return nil, status.Error(codes.FailedPrecondition, "inactive route or source IP mismatch")
	}
	return &corev1.PacketResult{Received: !lost, Lost: lost, LatencyMs: latency, Counters: counters}, nil
}
func (s *Service) GetStats(ctx context.Context, r *corev1.StatsRequest) (*corev1.Counters, error) {
	if r.SessionId != "" {
		if err := domain.IDs(r.SessionId); err != nil {
			return nil, err
		}
	}
	result := &corev1.Counters{}
	err := s.Store.Run(ctx, func(tx *store.Tx) error {
		routes, err := store.All[corev1.Route](tx, "routes")
		if err != nil {
			return err
		}
		found := r.SessionId == ""
		for _, route := range routes {
			if r.SessionId != "" && route.SessionId != r.SessionId {
				continue
			}
			found = true
			c := route.Counters
			if c == nil {
				continue
			}
			result.PacketsSent += c.PacketsSent
			result.PacketsReceived += c.PacketsReceived
			result.PacketsLost += c.PacketsLost
			result.PacketsRejected += c.PacketsRejected
			result.BytesSent += c.BytesSent
			result.BytesReceived += c.BytesReceived
			result.BytesLost += c.BytesLost
			result.BytesRejected += c.BytesRejected
		}
		if !found {
			return status.Error(codes.NotFound, "route not found")
		}
		return nil
	})
	return result, domain.RPCError(err)
}
