package service

import (
	"context"

	grpcd "git.sonicoriginal.software/grpcd-protos"
	"git.sonicoriginal.software/grpcd/internal"

	"git.sonicoriginal.software/grpc-foundation/errors"

	"git.sonicoriginal.software/logger"
	"google.golang.org/grpc/peer"
)

const (
	errCodeDeregistrationFailed = "DEREGISTRATION_FAILED"
)

// Deregister removes a service instance
func (s *GRPCDServer) Deregister(
	ctx context.Context, _ *grpcd.DeregisterRequest,
) (*grpcd.DeregisterResponse, error) {
	log := logger.FromContext(ctx)

	// Extract real address from gRPC peer context (cannot be spoofed)
	p, ok := peer.FromContext(ctx)
	if !ok {
		log.Error("Failed to extract peer info from context")
		return nil, errors.Internal(
			ctx,
			"failed to extract connection info",
			ErrCodePeerInfoUnavailable,
			internal.ErrDomain,
		)
	}
	address := p.Addr.String()

	log.Info("Deregistering service instance", "address", address)

	// Delete all methods for this address
	err := s.store.DeleteMethodsByAddress(ctx, address)
	if err != nil {
		log.Error("Failed to deregister service instance",
			"address", address,
			"error", err)
		return nil, errors.Internal(
			ctx,
			"failed to deregister service",
			errCodeDeregistrationFailed,
			internal.ErrDomain,
		)
	}

	log.Info("Successfully deregistered service instance", "address", address)

	// Update metrics
	if s.deregistrationCount != nil {
		s.deregistrationCount.Add(ctx, 1)
	}

	return &grpcd.DeregisterResponse{}, nil
}
