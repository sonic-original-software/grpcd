package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-testing/mocks/meter"
)

func TestNewGRPCDServer(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	if server == nil {
		t.Fatal("expected server, got nil")
	}
	if server.log == nil {
		t.Fatal("expected logger to be set")
	}
	if server.registrationCount == nil {
		t.Fatal("expected registrationCount to be set")
	}
	if server.deregistrationCount == nil {
		t.Fatal("expected deregistrationCount to be set")
	}
	if server.methodsDiscovered == nil {
		t.Fatal("expected methodsDiscovered to be set")
	}
}

func TestNewGRPCDServer_NilLogger(t *testing.T) {
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(nil, store, meter)

	if server == nil {
		t.Fatal("expected server, got nil")
	}
	if server.log == nil {
		t.Fatal("expected logger to be set even when nil passed")
	}
}

// TestNewGRPCDServer_NilStore validates that NewGRPCDServer creates a mock
// store when nil is passed, ensuring the server never operates with a nil store.
func TestNewGRPCDServer_NilStore(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	server := NewGRPCDServer(log, nil, meter)

	if server == nil {
		t.Fatal("expected server, got nil")
	}
	if server.store == nil {
		t.Fatal("expected store to be set even when nil passed")
	}
}

func TestNewGRPCDServer_MetricCreationErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()

	// Inject error for Int64Counter creation
	meter.SetInt64CounterError(errors.New("metric creation failed"))

	// Should still create server, but metrics will be nil and error logged
	server := NewGRPCDServer(log, store, meter)

	if server == nil {
		t.Fatal("expected server, got nil")
	}
	// Server should still be usable even if metric creation fails
	if server.log == nil {
		t.Fatal("expected logger to be set")
	}
}
