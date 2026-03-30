package service

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal"
	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/validate"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpc-foundation/errors"
	"git.sonicoriginal.software/logger"
)

const (
	errCodeDiscoverFailed = "DISCOVER_FAILED"
)

// validateDiscoverRequest validates a DiscoverRequest
func validateDiscoverRequest(req *grpcd.DiscoverRequest) []errors.FieldViolation {
	return validate.MethodName(req.MethodName)
}

// Discover finds the address for a specific method
func (s *GRPCDServer) Discover(
	ctx context.Context, req *grpcd.DiscoverRequest,
) (*grpcd.DiscoverResponse, error) {
	log := logger.FromContext(ctx)

	// Validate request
	violations := validateDiscoverRequest(req)
	if len(violations) > 0 {
		return nil, errors.InvalidArgument(ctx, "validation failed", violations...)
	}

	log.Info("Discovering method", "method_name", req.MethodName)

	// Lookup method → address
	address, err := s.store.GetMethodAddress(ctx, req.MethodName)
	if err == storage.ErrMethodNotFound {
		log.Info("Method not found", "method_name", req.MethodName)
		return nil, errors.NotFound(ctx, "method", req.MethodName)
	}
	if err != nil {
		log.Error("Failed to discover method",
			"method_name", req.MethodName,
			"error", err)
		return nil, errors.Internal(
			ctx,
			"failed to discover method",
			errCodeDiscoverFailed,
			internal.ErrDomain,
		)
	}

	log.Info("Discover successful",
		"method_name", req.MethodName,
		"address", address)

	// Update metrics
	if s.methodsDiscovered != nil {
		s.methodsDiscovered.Add(ctx, 1)
	}

	return &grpcd.DiscoverResponse{Address: address}, nil
}
