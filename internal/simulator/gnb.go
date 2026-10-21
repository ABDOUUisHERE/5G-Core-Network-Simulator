package simulator

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
)

type GNB struct {
	corev1.UnimplementedGNBServiceServer
	AMF corev1.AMFServiceClient
	UPF corev1.UPFServiceClient
}

func (s *GNB) RegisterUE(ctx context.Context, r *corev1.RegisterUERequest) (*corev1.RegisterUEResponse, error) {
	return s.AMF.RegisterUE(ctx, r)
}
func (s *GNB) DeregisterUE(ctx context.Context, r *corev1.UERequest) (*corev1.UE, error) {
	return s.AMF.DeregisterUE(ctx, r)
}
func (s *GNB) CreateSession(ctx context.Context, r *corev1.CreateSessionRequest) (*corev1.Session, error) {
	return s.AMF.CreateSession(ctx, r)
}
func (s *GNB) SendPacket(ctx context.Context, r *corev1.Packet) (*corev1.PacketResult, error) {
	return s.UPF.SendPacket(ctx, r)
}
