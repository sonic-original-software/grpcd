// Package cli assembles and runs the grpcd server.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"git.sonicoriginal.software/logger"

	foundationotel "git.sonicoriginal.software/grpc-foundation/otel"
	foundation "git.sonicoriginal.software/grpc-foundation/server"
	"git.sonicoriginal.software/grpc-service/diagnostics"
	service_lib "git.sonicoriginal.software/grpc-service/service"
	grpcd "github.com/grpcd/protos"

	"github.com/grpcd/server/internal/service"
	"github.com/grpcd/server/internal/storage"
	"github.com/grpcd/server/internal/storage/resolver"
)

// defaultServerName identifies this server when GRPC_SERVER_NAME is unset
const defaultServerName = "grpcd"

// cleanupTimeout bounds stopping the server and flushing telemetry, together
const cleanupTimeout = 5 * time.Second

// maxConnectionAge reports the age at which the server sends a GOAWAY, which is
// how long a registration can last regardless of the stream holding it. Zero is
// gRPC's infinity.
func maxConnectionAge() string {
	if value := os.Getenv(foundation.EnvMaxConnectionAge); value != "" {
		return value
	}

	return foundation.DefaultMaxConnectionAge.String()
}

// reportStoreHealth keeps the grpcd service's health entry matching the
// store's condition, for as long as ctx lives.
func reportStoreHealth(ctx context.Context, store storage.Store, healthSrv *health.Server) {
	for {
		condition := store.Condition()

		status := grpc_health_v1.HealthCheckResponse_SERVING
		if condition.Lost {
			status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
		}

		healthSrv.SetServingStatus(grpcd.GRPCDService_ServiceDesc.ServiceName, status)

		select {
		case <-condition.Changed:
		case <-ctx.Done():
			return
		}
	}
}

// Run serves until a signal arrives or serving fails, and answers with the
// process exit code. Every return runs the deferred teardown on its way out.
func Run() int {
	// The process context. The shutdown builds its deadline on this one, which
	// is why the signal cancels a child of it rather than this.
	ctx := context.Background()

	serveCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverName := foundation.Name(defaultServerName)

	log, flush, err := foundationotel.Init(ctx, serverName, foundation.Version())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize otel: %v\n", err)

		return 1
	}

	ctx = logger.ContextWithLogger(ctx, log)

	// Registrations are streams this process holds, so what it anchors dies with
	// it. The id names a channel for the life of the process and is never
	// referenced afterward, so it needs no coordination and no durability.
	anchor := uuid.NewString()

	log.InfoContext(ctx, "Registrations expire at the maximum connection age",
		slog.String(foundation.EnvMaxConnectionAge, maxConnectionAge()))

	srv := foundation.New(log)

	// Deferred before anything else can fail, so every path out of here stops
	// the server and exports what it logged on the way.
	defer foundation.HandleGracefulShutdown(ctx, log, srv, flush, cleanupTimeout)

	store, err := resolver.Resolve(ctx, log)
	if err != nil {
		log.Error("Could not resolve storage backend", slog.Any("error", err))

		return 1
	}

	grpcdServer := service.NewGRPCDServer(log, store, otel.Meter(serverName), anchor)

	lis, err := foundation.Listen()
	if err != nil {
		log.Error("Failed to create listener", slog.Any("error", err))

		return 1
	}

	log = log.With(slog.String("address", lis.Addr().String()))

	checks := diagnostics.Checks{service.StorageCheckName: grpcdServer.StorageCheck}

	healthSrv := health.NewServer()

	_, err = service_lib.Register(srv, healthSrv, checks, func(s grpc.ServiceRegistrar) {
		grpcd.RegisterGRPCDServiceServer(s, grpcdServer)
	})
	if err != nil {
		log.Error("Failed to register services", slog.Any("error", err))

		return 1
	}

	// The store's reachability is this service's health. Whatever routes to
	// this instance can ask and decide whether to keep sending clients; the
	// handlers hold what they have and wait for the store either way. The ""
	// entry stays SERVING: the process is alive.
	go reportStoreHealth(serveCtx, store, healthSrv)

	// A client that cannot reach an address has it removed, and that client may
	// be wrong. This watches for removals of addresses this process anchors and
	// writes them back, because holding their registration stream is proof the
	// service is up.
	removals, err := store.Watch(serveCtx, anchor)
	if err != nil {
		log.Error("Failed to watch for removals", slog.Any("error", err))

		return 1
	}

	go func() {
		for removal := range removals {
			grpcdServer.Reinstate(serveCtx, removal)
		}
	}()

	// Serve blocks, and a deferred teardown cannot run while it does, so it goes
	// to a goroutine and the select below decides when this returns.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()

	log.Info("gRPC server listening")

	select {
	case err := <-serveErr:
		if err != nil {
			log.Error("Failed to serve", slog.Any("error", err))

			return 1
		}
	case <-serveCtx.Done():
	}

	return 0
}
