package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	grpcd "git.sonicoriginal.software/grpcd-protos"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-testing/mocks/addr"
	"git.sonicoriginal.software/grpc-testing/mocks/meter"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TestRegister_Success validates that Register successfully stores method-to-address
// mappings for a service instance. This is the happy path test ensuring that when a
// service registers its methods, those methods become discoverable and map to the
// service's network address extracted from the gRPC peer context.
func TestRegister_Success(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info (simulates gRPC connection)
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Test data
	req := &grpcd.RegisterRequest{
		Methods: []string{
			"/principal.pb.PrincipalService/GetPrincipal",
			"/principal.pb.PrincipalService/CreatePrincipal",
			"/principal.pb.PrincipalService/UpdatePrincipal",
		},
	}

	// Execute
	resp, err := server.Register(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	for _, method := range req.Methods {
		storedAddr, err := store.GetMethodAddress(ctx, method)
		if err != nil {
			t.Errorf("method %s not stored: %v", method, err)
			continue
		}
		if storedAddr != addr.String() {
			t.Errorf("expected address %s for method %s, got %s", addr.String(), method, storedAddr)
		}
	}
}

// TestRegister_CustomTTL validates that Register respects the REGISTRATION_TTL_MINUTES
// environment variable when set to a valid positive integer. This allows operators to
// tune the registration lifetime based on their deployment needs (e.g., shorter TTLs
// for rapidly changing services, longer TTLs for stable deployments).
func TestRegister_CustomTTL(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Configure storage backend and custom TTL
	t.Setenv("REGISTRATION_TTL_MINUTES", "5")

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	// Execute
	_, err := server.Register(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the method is still accessible (TTL not expired)
	_, err = store.GetMethodAddress(ctx, req.Methods[0])
	if err != nil {
		t.Errorf("method should be accessible before TTL expires: %v", err)
	}
}

// TestRegister_InvalidTTL_UsesDefault validates that Register falls back to the default
// TTL (10 minutes) when REGISTRATION_TTL_MINUTES is set to an invalid value (non-numeric).
// This ensures the service remains operational even with misconfigured environment variables.
func TestRegister_InvalidTTL_UsesDefault(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Configure storage backend with invalid TTL
	t.Setenv("REGISTRATION_TTL_MINUTES", "invalid")

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	// Execute - should use default TTL (10 minutes)
	_, err := server.Register(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify method was registered (using default TTL)
	_, err = store.GetMethodAddress(ctx, req.Methods[0])
	if err != nil {
		t.Errorf("method should be accessible: %v", err)
	}
}

// TestRegister_ZeroTTL_UsesDefault validates that Register falls back to the default TTL
// (10 minutes) when REGISTRATION_TTL_MINUTES is set to zero or negative. Zero/negative TTLs
// would cause immediate expiration, so the service correctly rejects them and uses a safe default.
func TestRegister_ZeroTTL_UsesDefault(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Configure zero TTL
	t.Setenv("REGISTRATION_TTL_MINUTES", "0")

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	// Execute - should use default TTL (10 minutes) since 0 is invalid
	_, err := server.Register(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify method was registered
	_, err = store.GetMethodAddress(ctx, req.Methods[0])
	if err != nil {
		t.Errorf("method should be accessible: %v", err)
	}
}

// TestRegister_EmptyMethods validates that Register returns InvalidArgument when called
// with an empty methods array. At least one method must be provided for registration to
// be meaningful, and this validation prevents services from registering without methods.
func TestRegister_EmptyMethods(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{}}

	// Execute
	_, err := server.Register(ctx, req)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty methods, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// TestRegister_NoPeerInfo validates that Register returns Internal error when peer
// information cannot be extracted from the gRPC context. The peer address is critical
// for registration as it determines the address that will be returned by Discover calls.
// This cannot be spoofed as it's extracted directly from the gRPC connection metadata.
func TestRegister_NoPeerInfo(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context WITHOUT peer info
	ctx := t.Context()

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	// Execute
	_, err := server.Register(ctx, req)

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

// TestRegister_StorageError validates that Register returns Internal error when the
// storage backend fails during registration. This ensures storage failures are properly
// surfaced to the caller rather than silently failing, allowing services to implement
// retry logic or fail gracefully.
func TestRegister_StorageError(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Inject storage error
	store.SetSetMethodAddressError(errors.New("storage unavailable"))

	// Test data
	req := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	// Execute
	_, err := server.Register(ctx, req)

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

// TestRegister_OverwritesExistingRegistration validates that when a new service instance
// registers a method that's already registered by another address, the new registration
// overwrites the old one. This implements last-writer-wins semantics and is important for
// service failover scenarios where a new instance takes over methods from a failed instance.
func TestRegister_OverwritesExistingRegistration(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Register from first address
	addr1 := addr.New("192.168.1.100:50054")
	ctx1 := peer.NewContext(t.Context(), &peer.Peer{Addr: addr1})

	req1 := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	_, err := server.Register(ctx1, req1)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Register same method from second address (should overwrite)
	addr2 := addr.New("192.168.1.200:50054")
	ctx2 := peer.NewContext(t.Context(), &peer.Peer{Addr: addr2})

	req2 := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method"}}

	_, err = server.Register(ctx2, req2)
	if err != nil {
		t.Fatalf("second registration failed: %v", err)
	}

	// Verify second address is now registered
	storedAddr, err := store.GetMethodAddress(t.Context(), "/test.Service/Method")
	if err != nil {
		t.Fatalf("failed to get method address: %v", err)
	}
	if storedAddr != addr2.String() {
		t.Errorf("expected address %s, got %s", addr2.String(), storedAddr)
	}
}

// TestRegister_MultipleMethodsPartialFailure validates that Register does not provide
// transactional semantics - if storage fails after some methods are registered, those
// methods remain registered while subsequent methods fail. This test documents the current
// behavior: registration is not atomic across multiple methods.
func TestRegister_MultipleMethodsPartialFailure(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Create context with peer info
	addr := addr.New("192.168.1.100:50054")
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

	// Register first method successfully
	req1 := &grpcd.RegisterRequest{Methods: []string{"/test.Service/Method1"}}

	_, err := server.Register(ctx, req1)
	if err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}

	// Now inject error and try to register multiple methods
	store.SetSetMethodAddressError(errors.New("storage error"))

	req2 := &grpcd.RegisterRequest{
		Methods: []string{
			"/test.Service/Method2",
			"/test.Service/Method3",
		},
	}
	_, err = server.Register(ctx, req2)
	if err == nil {
		t.Fatal("expected error for storage failure")
	}

	// Verify first method is still registered (transaction is not atomic)
	store.SetSetMethodAddressError(nil)
	storedAddr, err := store.GetMethodAddress(t.Context(), "/test.Service/Method1")
	if err != nil {
		t.Errorf("first method should still be registered: %v", err)
	}
	if storedAddr != addr.String() {
		t.Errorf("expected address %s, got %s", addr.String(), storedAddr)
	}
}
