//revive:disable:package-comments
package service

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"git.sonicoriginal.software/logger"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"
)

const (
	// ErrCodePeerInfoUnavailable denotes an error when the peer info is unavailable
	ErrCodePeerInfoUnavailable = "PEER_INFO_UNAVAILABLE"
)

// GRPCDServer implements the GRPCDService
type GRPCDServer struct {
	grpcd.UnimplementedGRPCDServiceServer
	log    *slog.Logger
	store  storage.Store
	anchor string

	// Business metrics
	registrationCount metric.Int64Counter
	removalCount      metric.Int64Counter
	methodsDiscovered metric.Int64Counter
	revertedRemovals  metric.Int64Counter
}

// NewGRPCDServer creates a new grpcd server.
//
// anchor identifies this instance for the life of the process. It is recorded
// on every address this server registers, so whoever removes one of those rows
// knows which instance to tell.
func NewGRPCDServer(
	log *slog.Logger, store storage.Store, meter metric.Meter, anchor string,
) *GRPCDServer {
	if log == nil {
		log = logger.NewNullLogger()
	}

	if store == nil {
		store = mock.NewStore()
	}

	// Initialize business metrics
	registrationCount, err := meter.Int64Counter(
		"grpcd.registrations.total",
		metric.WithDescription("Total number of service registrations"),
		metric.WithUnit("{registration}"),
	)
	if err != nil {
		log.Error("Failed to create registrations metric", "error", err)
	}

	removalCount, err := meter.Int64Counter(
		"grpcd.removals.total",
		metric.WithDescription("Total number of address removals"),
		metric.WithUnit("{removal}"),
	)
	if err != nil {
		log.Error("Failed to create removals metric", "error", err)
	}

	methodsDiscovered, err := meter.Int64Counter(
		"grpcd.discoveries.total",
		metric.WithDescription("Total number of method discoveries"),
		metric.WithUnit("{grpcd}"),
	)
	if err != nil {
		log.Error("Failed to create discoveries metric", "error", err)
	}

	revertedRemovals, err := meter.Int64Counter(
		"grpcd.removals.reverted.total",
		metric.WithDescription(
			"Total number of removals written back because this instance still holds the stream",
		),
		metric.WithUnit("{removal}"),
	)
	if err != nil {
		log.Error("Failed to create reverted removals metric", "error", err)
	}

	return &GRPCDServer{
		log:               log,
		store:             store,
		anchor:            anchor,
		registrationCount: registrationCount,
		removalCount:      removalCount,
		methodsDiscovered: methodsDiscovered,
		revertedRemovals:  revertedRemovals,
	}
}
