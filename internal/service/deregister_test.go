package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	grpcd "git.sonicoriginal.software/grpcd-protos"
	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-testing/mocks/addr"
	"git.sonicoriginal.software/grpc-testing/mocks/meter"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TestDeregister_Success validates that Deregister successfully removes all methods
// registered by a service instance. This is the happy path test ensuring that when
// a service calls Deregister, all method-to-address mappings for that address are
// removed from storage, making those methods unavailable for discovery.
func TestDeregister_Success(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Seed data: Register methods first
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	methods := []string{
		"/principal.pb.PrincipalService/GetPrincipal",
		"/principal.pb.PrincipalService/CreatePrincipal",
		"/principal.pb.PrincipalService/UpdatePrincipal",
	}
	for _, method := range methods {
		err := store.SetMethodAddress(ctx, method, addr.String(), 10*time.Minute)
		if err != nil {
			t.Fatalf("failed to seed method %s: %v", method, err)
		}
	}

	// Verify methods are registered
	for _, method := range methods {
		_, err := store.GetMethodAddress(ctx, method)
		if err != nil {
			t.Fatalf("method %s should be registered before deregister: %v", method, err)
		}
	}

	// Execute
	req := &grpcd.DeregisterRequest{}
	resp, err := server.Deregister(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	// Verify all methods are removed
	for _, method := range methods {
		_, err := store.GetMethodAddress(ctx, method)
		if err != storage.ErrMethodNotFound {
			t.Errorf("method %s should be removed, got error: %v", method, err)
		}
	}
}

// TestDeregister_NoMethodsRegistered validates that Deregister is idempotent when
// called by an address that has no registered methods. This ensures services can
// safely call Deregister without checking if they have any registrations, and that
// deregistering an already-deregistered service doesn't cause errors.
func TestDeregister_NoMethodsRegistered(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// DO NOT seed any methods - this address has no registrations

	// Execute
	req := &grpcd.DeregisterRequest{}
	resp, err := server.Deregister(ctx, req)

	// Assert - should succeed even if nothing to deregister (idempotent)
	if err != nil {
		t.Fatalf("expected no error for idempotent deregister, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

// TestDeregister_OnlyRemovesOwnMethods validates that Deregister only removes methods
// registered by the calling address and does not affect methods registered by other
// service instances. This is critical for multi-instance deployments where multiple
// services may handle the same or different methods, ensuring deregistration is properly
// isolated to the calling instance.
func TestDeregister_OnlyRemovesOwnMethods(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Seed data: Register methods from two different addresses
	addr1 := addr.New("192.168.1.100:50054")
	addr2 := addr.New("192.168.1.200:50054")

	methods1 := []string{
		"/principal.pb.PrincipalService/GetPrincipal",
		"/principal.pb.PrincipalService/CreatePrincipal",
	}
	methods2 := []string{
		"/storage.pb.StorageService/GetObject",
		"/storage.pb.StorageService/PutObject",
	}

	for _, method := range methods1 {
		err := store.SetMethodAddress(t.Context(), method, addr1.String(), 10*time.Minute)
		if err != nil {
			t.Fatalf("failed to seed method %s for addr1: %v", method, err)
		}
	}
	for _, method := range methods2 {
		err := store.SetMethodAddress(t.Context(), method, addr2.String(), 10*time.Minute)
		if err != nil {
			t.Fatalf("failed to seed method %s for addr2: %v", method, err)
		}
	}

	// Deregister from addr1
	ctx1 := peer.NewContext(t.Context(), &peer.Peer{Addr: addr1})
	req := &grpcd.DeregisterRequest{}
	_, err := server.Deregister(ctx1, req)
	if err != nil {
		t.Fatalf("deregister failed: %v", err)
	}

	// Verify addr1 methods are removed
	for _, method := range methods1 {
		_, err := store.GetMethodAddress(t.Context(), method)
		if err != storage.ErrMethodNotFound {
			t.Errorf("method %s from addr1 should be removed, got error: %v", method, err)
		}
	}

	// Verify addr2 methods are still present
	for _, method := range methods2 {
		storedAddr, err := store.GetMethodAddress(t.Context(), method)
		if err != nil {
			t.Errorf("method %s from addr2 should still be registered: %v", method, err)
		}
		if storedAddr != addr2.String() {
			t.Errorf("method %s should still point to addr2 %s, got %s", method, addr2.String(), storedAddr)
		}
	}
}

// TestDeregister_NoPeerInfo validates that Deregister returns Internal error when
// peer information cannot be extracted from the gRPC context. The peer address is
// required to identify which service instance is deregistering, and this cannot be
// spoofed as it's extracted directly from the gRPC connection metadata.
func TestDeregister_NoPeerInfo(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context WITHOUT peer info
	ctx := t.Context()

	// Execute
	req := &grpcd.DeregisterRequest{}
	_, err := server.Deregister(ctx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing peer info, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}

// TestDeregister_StorageError validates that Deregister returns Internal error when
// the storage backend fails during deregistration. This ensures storage failures are
// properly surfaced to the caller rather than silently failing or returning success.
func TestDeregister_StorageError(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Inject storage error BEFORE calling Deregister
	store.SetDeleteMethodsByAddressError(errors.New("storage unavailable"))

	// Execute
	req := &grpcd.DeregisterRequest{}
	_, err := server.Deregister(ctx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for storage failure, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}

// TestDeregister_MultipleCallsIdempotent validates that calling Deregister multiple
// times from the same address succeeds without error. This is important for reliable
// service shutdown where a service might call Deregister multiple times (e.g., in
// signal handlers, error paths, graceful shutdown sequences) without needing to track
// whether it has already deregistered.
func TestDeregister_MultipleCallsIdempotent(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Seed data: Register methods first
	methods := []string{
		"/test.Service/Method1",
		"/test.Service/Method2",
	}
	for _, method := range methods {
		err := store.SetMethodAddress(ctx, method, addr.String(), 10*time.Minute)
		if err != nil {
			t.Fatalf("failed to seed method %s: %v", method, err)
		}
	}

	// First deregister
	req := &grpcd.DeregisterRequest{}
	_, err := server.Deregister(ctx, req)
	if err != nil {
		t.Fatalf("first deregister failed: %v", err)
	}

	// Second deregister (should be idempotent)
	_, err = server.Deregister(ctx, req)
	if err != nil {
		t.Fatalf("second deregister should be idempotent, got error: %v", err)
	}

	// Third deregister (still idempotent)
	_, err = server.Deregister(ctx, req)
	if err != nil {
		t.Fatalf("third deregister should be idempotent, got error: %v", err)
	}

	// Verify methods are still removed
	for _, method := range methods {
		_, err := store.GetMethodAddress(ctx, method)
		if err != storage.ErrMethodNotFound {
			t.Errorf("method %s should still be removed, got error: %v", method, err)
		}
	}
}
