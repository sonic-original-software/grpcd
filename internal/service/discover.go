package service

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"git.sonicoriginal.software/logger"

	foundationerrors "git.sonicoriginal.software/grpc-foundation/errors"
	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpcd/internal"
	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/validate"
)

const (
	errCodeDiscoverFailed = "DISCOVER_FAILED"
)

// Discover answers with the addresses serving a method, one at a time.
//
// The caller takes the first it can reach and closes the stream. One it cannot
// reach it reports back, and that address is removed before the next is sent,
// so the set converges on what is actually reachable without grpcd checking
// anything itself.
//
// When there is nothing left to offer, the stream is held and the next address
// registered for the method is offered as it arrives. A caller whose backend is
// entirely down blocks on a receive rather than asking again, and is woken by
// the registration.
func (s *GRPCDServer) Discover(stream grpcd.GRPCDService_DiscoverServer) error {
	ctx := stream.Context()
	log := logger.FromContext(ctx)

	method, err := methodName(ctx, stream)
	if err != nil {
		return err
	}

	log = log.With("method_name", method)
	log.InfoContext(ctx, "Discovering method")

	sent := 0

	for address, err := range s.store.AddressesFor(ctx, method) {
		if err != nil {
			log.ErrorContext(ctx, "Failed to discover method", "error", err)

			return foundationerrors.Internal(
				ctx, "failed to discover method", errCodeDiscoverFailed, internal.ErrDomain,
			)
		}

		sent++

		if done, err := s.offer(ctx, log, stream, method, address); done || err != nil {
			return err
		}
	}

	log.InfoContext(ctx, "No addresses left for method, waiting", "candidates_sent", sent)

	// Subscribed once, here, and held until this returns — so an address added
	// between a refusal and the next wait is not missed.
	additions, err := s.store.WatchMethod(ctx, method)
	if err != nil {
		log.ErrorContext(ctx, "Failed to wait for method", "error", err)

		return foundationerrors.Internal(
			ctx, "failed to discover method", errCodeDiscoverFailed, internal.ErrDomain,
		)
	}

	// The store closes this when ctx ends, so the range is what ends the wait.
	for address := range additions {
		if done, err := s.offer(ctx, log, stream, method, address); done || err != nil {
			return err
		}
	}

	return ctx.Err()
}

// offer sends address as a candidate and waits for the caller's verdict. It
// answers done when the caller closed the stream, which is how a caller says
// the candidate worked.
func (s *GRPCDServer) offer(
	ctx context.Context,
	log *slog.Logger,
	stream grpcd.GRPCDService_DiscoverServer,
	method, address string,
) (bool, error) {
	if err := stream.Send(&grpcd.DiscoverResponse{Address: address}); err != nil {
		return false, err
	}

	reported, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		if s.methodsDiscovered != nil {
			s.methodsDiscovered.Add(ctx, 1)
		}

		log.InfoContext(ctx, "Discover successful", "method_address", address)

		return true, nil
	}

	if err != nil {
		return false, err
	}

	s.reportedDead(ctx, log, method, reported.GetDeadAddress())

	return false, nil
}

// methodName reads the method off the stream's first message.
func methodName(ctx context.Context, stream grpcd.GRPCDService_DiscoverServer) (string, error) {
	request, err := stream.Recv()
	if err != nil {
		return "", err
	}

	method := request.GetMethodName()

	if violations := validate.MethodName(method); len(violations) > 0 {
		return "", foundationerrors.InvalidArgument(ctx, "validation failed", violations...)
	}

	return method, nil
}

// reportedDead removes an address the caller could not reach and tells the
// instance anchoring it, which writes the row back if it still holds that
// address's registration stream.
func (s *GRPCDServer) reportedDead(
	ctx context.Context, log *slog.Logger, method, address string,
) {
	if address == "" {
		return
	}

	log = log.With("method_address", address)

	anchor, err := s.store.RemoveFromMethod(ctx, method, address)
	if err != nil {
		log.ErrorContext(ctx, "Failed to remove unreachable address", "error", err)
		return
	}

	if s.removalCount != nil {
		s.removalCount.Add(ctx, 1)
	}

	log.InfoContext(ctx, "Removed unreachable address")

	if anchor == "" {
		return
	}

	removal := storage.Removal{Method: method, Address: address}

	if err := s.store.Notify(ctx, anchor, removal); err != nil {
		log.ErrorContext(ctx, "Failed to notify the anchoring instance", "error", err)
	}
}

// Reinstate writes back a row removed from under this instance's anchor.
//
// The store publishes only to the anchor recorded on the row, and that anchor
// is gone once the registration stream ends, so a notification arriving means
// this instance held the stream when the removal happened. That is proof enough
// to write it back without checking anything.
//
// A stream that ended in the meantime leaves a row for a service that is gone,
// and the next client to fail against it removes it again.
func (s *GRPCDServer) Reinstate(ctx context.Context, removal storage.Removal) {
	log := s.log.With("peer_address", removal.Address, "method_name", removal.Method)

	if err := s.store.Add(ctx, removal.Address, s.anchor, []string{removal.Method}); err != nil {
		log.ErrorContext(ctx, "Failed to reinstate address", "error", err)
		return
	}

	if s.revertedRemovals != nil {
		s.revertedRemovals.Add(ctx, 1)
	}

	log.InfoContext(ctx, "Reinstated address removed while its stream is held")
}
