//revive:disable:package-comments
package service

import (
	"log/slog"

	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpc-protos/diagnostics"
	"git.sonicoriginal.software/grpc-protos/info"

	"git.sonicoriginal.software/logger"

	"go.opentelemetry.io/otel/metric"
)

const (
	// ErrCodePeerInfoUnavailable denotes an error when the peer info is unavailable
	ErrCodePeerInfoUnavailable = "PEER_INFO_UNAVAILABLE"
)

// GRPCDServer implements the GRPCDService
type GRPCDServer struct {
	info.UnimplementedInfoServiceServer
	diagnostics.UnimplementedDiagnosticsServiceServer

	grpcd.UnimplementedGRPCDServiceServer
	log   *slog.Logger
	store storage.Store

	// Business metrics
	registrationCount   metric.Int64Counter
	deregistrationCount metric.Int64Counter
	methodsDiscovered   metric.Int64Counter
}

// NewGRPCDServer creates a new grpcd server
func NewGRPCDServer(
	log *slog.Logger, store storage.Store, meter metric.Meter,
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

	deregistrationCount, err := meter.Int64Counter(
		"grpcd.deregistrations.total",
		metric.WithDescription("Total number of service deregistrations"),
		metric.WithUnit("{deregistration}"),
	)
	if err != nil {
		log.Error("Failed to create deregistrations metric", "error", err)
	}

	methodsDiscovered, err := meter.Int64Counter(
		"grpcd.discoveries.total",
		metric.WithDescription("Total number of method discoveries"),
		metric.WithUnit("{grpcd}"),
	)
	if err != nil {
		log.Error("Failed to create discoveries metric", "error", err)
	}

	return &GRPCDServer{
		log:                 log,
		store:               store,
		registrationCount:   registrationCount,
		deregistrationCount: deregistrationCount,
		methodsDiscovered:   methodsDiscovered,
	}
}
