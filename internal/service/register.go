package service

import (
	"context"
	"os"
	"strconv"
	"time"

	"git.sonicoriginal.software/grpcd/internal"
	"git.sonicoriginal.software/grpcd/internal/validate"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpc-foundation/errors"

	"git.sonicoriginal.software/logger"

	"google.golang.org/grpc/peer"
)

const (
	errCodeRegistrationFailed = "REGISTRATION_FAILED"
)

// validateRegisterRequest validates a RegisterRequest
func validateRegisterRequest(req *grpcd.RegisterRequest) []errors.FieldViolation {
	return validate.Methods(req.Methods)
}

// Register registers a new service instance
func (s *GRPCDServer) Register(
	ctx context.Context, req *grpcd.RegisterRequest,
) (*grpcd.RegisterResponse, error) {
	log := logger.FromContext(ctx)

	// Validate request
	violations := validateRegisterRequest(req)
	if len(violations) > 0 {
		return nil, errors.InvalidArgument(ctx, "validation failed", violations...)
	}

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
	methodCount := len(req.Methods)

	log.Info("Registering service instance",
		"address", address,
		"method_count", methodCount)

	log.Debug("Registering methods", "methods", req.Methods)

	// Read TTL from environment variable (hot-read, no restart needed)
	ttlMinutes := 10 // default
	if ttlEnv := os.Getenv("REGISTRATION_TTL_MINUTES"); ttlEnv != "" {
		if parsed, err := strconv.Atoi(ttlEnv); err == nil && parsed > 0 {
			ttlMinutes = parsed
		} else {
			log.Warn("Invalid REGISTRATION_TTL_MINUTES, using default",
				"value", ttlEnv,
				"default", ttlMinutes)
		}
	}
	ttl := time.Duration(ttlMinutes) * time.Minute

	// Store each method → address mapping with TTL
	for _, method := range req.Methods {
		err := s.store.SetMethodAddress(ctx, method, address, ttl)
		if err != nil {
			log.Error("Failed to register method",
				"method", method,
				"address", address,
				"error", err)
			return nil, errors.Internal(
				ctx,
				"failed to register service",
				errCodeRegistrationFailed,
				internal.ErrDomain,
			)
		}
	}

	log.Info("Successfully registered service instance",
		"address", address,
		"method_count", methodCount,
		"ttl_minutes", ttlMinutes)

	// Update metrics
	if s.registrationCount != nil {
		s.registrationCount.Add(ctx, 1)
	}

	return &grpcd.RegisterResponse{}, nil
}
