//revive:disable:package-comments
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	grpcdclient "git.sonicoriginal.software/grpcd-go/client"
	"git.sonicoriginal.software/grpcd-go/diagnostics"
	service_lib "git.sonicoriginal.software/grpcd-go/service"
	"git.sonicoriginal.software/grpcd/internal/service"
	"git.sonicoriginal.software/grpcd/internal/storage/resolver"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	foundationotel "git.sonicoriginal.software/grpc-foundation/otel"
	foundation "git.sonicoriginal.software/grpc-foundation/server"

	"git.sonicoriginal.software/logger"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

// defaultServerName identifies this server when GRPC_SERVER_NAME is unset
const defaultServerName = "grpcd"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverName := foundation.Name(defaultServerName)

	log, shutdown, err := foundationotel.Init(ctx, serverName, foundation.Version())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize otel: %v\n", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	// Create OpenTelemetry meter
	meter := otel.Meter(serverName)

	store, err := resolver.Resolve(ctx, log)
	if err != nil {
		log.Error("Could not resolve storage backend", slog.Any("error", err))
		os.Exit(1)
	}

	grpcdServer := service.NewGRPCDServer(log, store, meter)

	lis, err := foundation.Listen()
	if err != nil {
		log.Error("Failed to create listener", slog.Any("error", err))
		os.Exit(1)
	}

	srv := foundation.New(log)

	log = log.With(slog.String("address", lis.Addr().String()))
	ctx = logger.ContextWithLogger(ctx, log)

	checks := diagnostics.Checks{service.StorageCheckName: grpcdServer.StorageCheck}

	healthSrv := health.NewServer()

	methodList, err := service_lib.Register(srv, healthSrv, checks, func(s grpc.ServiceRegistrar) {
		grpcd.RegisterGRPCDServiceServer(s, grpcdServer)
	})
	if err != nil {
		log.Error("Failed to register services", slog.Any("error", err))
		os.Exit(1)
	}

	grpcdClient := grpcdclient.New(log, methodList)
	go grpcdClient.Run(ctx)

	go foundation.HandleGracefulShutdown(ctx, cancel, log, srv, 5*time.Second)

	// Start gRPC server (blocking)
	if err := srv.Serve(lis); err != nil {
		log.Error("Failed to serve", slog.Any("error", err))
		os.Exit(1)
	}
}
