package service

import (
	"git.sonicoriginal.software/logger"

	"git.sonicoriginal.software/grpc-foundation/errors"
	grpcd "github.com/grpcd/protos"

	"github.com/grpcd/server/internal"
	"github.com/grpcd/server/internal/validate"
)

const (
	errCodeWatchFailed = "WATCH_FAILED"
)

// validateWatchRequest validates a WatchRequest
func validateWatchRequest(req *grpcd.WatchRequest) []errors.FieldViolation {
	violations := validate.MethodName(req.MethodName)

	if req.Address == "" {
		violations = append(violations, errors.FieldViolation{
			Field:       "address",
			Description: "address is required",
		})
	}

	return violations
}

// Watch holds the stream for a client that has an address for a method, and
// tells it when to move to a newly registered one.
//
// Every registration wakes this. One for the client's method at an address
// other than the one it holds is a candidate to move to, and the client is
// told with probability 1/N, N being the addresses serving the method after
// the addition: each holder decides alone, and the new replica's expected
// share of them is exactly its fair share. The stream is held until the
// client closes it, which it does once it has moved and opened a new Watch
// naming where it went.
func (s *GRPCDServer) Watch(req *grpcd.WatchRequest, stream grpcd.GRPCDService_WatchServer) error {
	ctx := stream.Context()
	log := logger.FromContext(ctx).With("method_name", req.MethodName, "held_address", req.Address)

	if violations := validateWatchRequest(req); len(violations) > 0 {
		return errors.InvalidArgument(ctx, "validation failed", violations...)
	}

	log.InfoContext(ctx, "Watching method")

	latest := s.store.Latest()

	for {
		select {
		case <-latest.Done:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Reloaded before anything else, so the next sleep is on the newest
		// announcement and one landing in between is not slept through.
		latest = s.store.Latest()

		if latest.Method != req.MethodName || latest.Address == req.Address {
			continue
		}

		n, err := s.store.Count(ctx, req.MethodName)
		if err != nil {
			lost, waited := storeLost(ctx, s.store)

			if !lost {
				log.ErrorContext(ctx, "Failed to count addresses", "error", err)

				return errors.Internal(ctx, "failed to watch method", errCodeWatchFailed, internal.ErrDomain)
			}

			if !waited {
				return ctx.Err()
			}

			// The addition that woke this is gone with the store; the next
			// one is caught now that it has returned.
			log.WarnContext(ctx, "Store returned, watching again", "error", err)

			continue
		}

		if !s.roll(n) {
			continue
		}

		if err := stream.Send(&grpcd.WatchResponse{Address: latest.Address}); err != nil {
			return err
		}

		if s.rebalanceCount != nil {
			s.rebalanceCount.Add(ctx, 1)
		}

		log.InfoContext(ctx, "Told to move", "address", latest.Address)
	}
}
