//revive:disable:package-comments
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	service_lib "git.sonicoriginal.software/grpcd-go/service"
	"git.sonicoriginal.software/grpcd/internal/service"
	"git.sonicoriginal.software/grpcd/internal/storage/resolver"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	foundationotel "git.sonicoriginal.software/grpc-foundation/otel"
	"git.sonicoriginal.software/grpc-protos/diagnostics"
	"git.sonicoriginal.software/grpc-protos/info"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
)

const serviceName = "grpcd"

func main() {
	ctx := context.Background()

	log, shutdown, err := foundationotel.Init(ctx, serviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize otel: %v\n", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	// Create OpenTelemetry meter
	meter := otel.Meter(serviceName)

	store, err := resolver.Resolve(ctx, log)
	if err != nil {
		log.Error("Could not resolve storage backend", slog.Any("error", err))
		os.Exit(1)
	}

	grpcdServer := service.NewGRPCDServer(log, store, meter)

	// Start gRPC server (blocking)
	err = service_lib.Run(
		ctx, serviceName, log,
		func(s grpc.ServiceRegistrar) {
			info.RegisterInfoServiceServer(s, grpcdServer)
			diagnostics.RegisterDiagnosticsServiceServer(s, grpcdServer)

			grpcd.RegisterGRPCDServiceServer(s, grpcdServer)
		},
		nil,
	)
	if err != nil {
		log.Error("Server failed", slog.Any("error", err))
		os.Exit(1)
	}
}
