package service

import (
	"context"
	"log/slog"
	"net"
	"strconv"

	"google.golang.org/grpc/peer"

	"git.sonicoriginal.software/logger"

	"git.sonicoriginal.software/grpc-foundation/errors"
	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpcd/internal"
	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/validate"
)

const (
	errCodeRegistrationFailed = "REGISTRATION_FAILED"
)

// validateRegisterRequest validates a RegisterRequest
func validateRegisterRequest(req *grpcd.RegisterRequest) []errors.FieldViolation {
	violations := validate.Methods(req.Methods)

	if req.Port == 0 || req.Port > 65535 {
		violations = append(violations, errors.FieldViolation{
			Field:       "port",
			Description: "port must be between 1 and 65535",
		})
	}

	return violations
}

// Register records the caller's methods and holds the stream open.
//
// The stream is the registration: the rows exist while it is held, and this
// handler removes them on its way out. A caller that crashes ends the stream
// the same way a caller that exits cleanly does, so both are the same path.
func (s *GRPCDServer) Register(
	req *grpcd.RegisterRequest, stream grpcd.GRPCDService_RegisterServer,
) error {
	ctx := stream.Context()
	log := logger.FromContext(ctx).With("server_name", req.ServerName)

	violations := validateRegisterRequest(req)
	if len(violations) > 0 {
		return errors.InvalidArgument(ctx, "validation failed", violations...)
	}

	address, err := s.address(ctx, req.Port)
	if err != nil {
		log.ErrorContext(ctx, "Failed to extract peer info from context")

		return err
	}

	log = log.With("peer_address", address, "method_count", len(req.Methods))

	log.InfoContext(ctx, "Registering service instance")
	log.DebugContext(ctx, "Registering methods", "methods", req.Methods)

	condition, err := s.write(ctx, log, address, req.Methods)
	if err != nil {
		return err
	}

	if err := stream.Send(&grpcd.RegisterResponse{}); err != nil {
		log.ErrorContext(ctx, "Failed to acknowledge registration", "error", err)
		s.release(ctx, log, address, req.Methods)

		return err
	}

	if s.registrationCount != nil {
		s.registrationCount.Add(ctx, 1)
	}

	log.InfoContext(ctx, "Successfully registered service instance")

	// Holding the stream is the registration. Returning ends it, so this waits
	// for the caller to go away. The store coming back in the meantime means
	// it may have come back empty, and the rows are written again from the
	// request this handler still holds.
	for {
		select {
		case <-ctx.Done():
			s.release(ctx, log, address, req.Methods)

			return nil
		case <-condition.Changed:
		}

		if condition = s.store.Condition(); condition.Lost {
			continue
		}

		if err := s.store.Add(ctx, address, s.anchor, req.Methods); err != nil {
			log.ErrorContext(ctx, "Failed to rewrite methods after the store came back", "error", err)

			continue
		}

		log.InfoContext(ctx, "Rewrote methods after the store came back")
	}
}

// write records the rows, waiting out a lost store rather than failing on it.
// It answers with the condition the rows were written under, for the holder
// to watch for the next change.
func (s *GRPCDServer) write(
	ctx context.Context, log *slog.Logger, address string, methods []string,
) (*storage.Condition, error) {
	for {
		// Read before the write, so a loss the write itself records closes
		// this one's Changed and the holder wakes to look.
		condition := s.store.Condition()

		err := s.store.Add(ctx, address, s.anchor, methods)
		if err == nil {
			return condition, nil
		}

		lost, waited := storeLost(ctx, s.store)

		if !lost {
			log.ErrorContext(ctx, "Failed to register methods", "error", err)

			return nil, errors.Internal(
				ctx, "failed to register service", errCodeRegistrationFailed, internal.ErrDomain,
			)
		}

		if !waited {
			return nil, ctx.Err()
		}

		log.WarnContext(ctx, "Store returned, registering again", "error", err)
	}
}

// release removes the rows this stream was holding.
//
// The context that ended the stream is already cancelled, so the removal runs
// on one detached from it. Nothing else will run this removal: the rows carry
// no expiry, and this instance is the only one watching this stream.
func (s *GRPCDServer) release(
	ctx context.Context, log *slog.Logger, address string, methods []string,
) {
	ctx = context.WithoutCancel(ctx)

	if err := s.store.Remove(ctx, address, methods); err != nil {
		log.ErrorContext(ctx, "Failed to remove methods", "error", err)

		return
	}

	if s.removalCount != nil {
		s.removalCount.Add(ctx, 1)
	}

	log.InfoContext(ctx, "Removed service instance")
}

// address composes the caller's address from the IP on the connection and the
// port the caller reported.
//
// Neither half is available on its own: a containerized service does not know
// its reachable IP, and the port on the peer socket is the ephemeral one the
// caller dialed from.
func (s *GRPCDServer) address(ctx context.Context, port uint32) (string, error) {
	info, ok := peer.FromContext(ctx)
	if !ok {
		return "", errors.Internal(
			ctx,
			"failed to extract connection info",
			ErrCodePeerInfoUnavailable,
			internal.ErrDomain,
		)
	}

	host, _, err := net.SplitHostPort(info.Addr.String())
	if err != nil {
		host = info.Addr.String()
	}

	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10)), nil
}
