package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-protos/diagnostics"
	"git.sonicoriginal.software/grpc-testing/mocks/meter"
)

func TestGetDiagnostics_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	resp, err := server.GetDiagnostics(t.Context(), &diagnostics.GetDiagnosticsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Services == nil {
		t.Error("expected services map to be initialized")
	}

	if len(resp.Services) != 1 {
		t.Errorf("expected 0 services, got %d", len(resp.Services))
	}
}

func TestGetDiagnostics_StoreError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	store.SetPingError(errors.New("expected failure"))
	server := NewGRPCDServer(log, store, meter)

	resp, err := server.GetDiagnostics(t.Context(), &diagnostics.GetDiagnosticsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Services == nil {
		t.Error("expected services map to be initialized")
	}

	if len(resp.Services) != 1 {
		t.Errorf("expected 0 services, got %d", len(resp.Services))
	}
}
