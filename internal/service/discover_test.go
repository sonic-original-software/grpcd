package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpc-testing/mocks/meter"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestDiscover_Success validates that Discover successfully returns the address for a
// registered method. This is the happy path ensuring that clients can look up service
// addresses for methods that have been registered via Register calls.
func TestDiscover_Success(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Seed data: Register a method
	methodName := "/principal.pb.PrincipalService/GetPrincipal"
	serviceAddr := "192.168.1.100:50054"
	err := store.SetMethodAddress(t.Context(), methodName, serviceAddr, 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to seed method: %v", err)
	}

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: methodName}
	resp, err := server.Discover(t.Context(), req)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Address != serviceAddr {
		t.Errorf("expected address %s, got %s", serviceAddr, resp.Address)
	}
}

// TestDiscover_MethodNotFound validates that Discover returns NotFound error when
// querying for a method that has not been registered. This allows clients to
// distinguish between "method doesn't exist" and other error conditions.
func TestDiscover_MethodNotFound(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// DO NOT seed any data - method is not registered

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: "/nonexistent.Service/Method"}
	_, err := server.Discover(t.Context(), req)

	// Assert
	if err == nil {
		t.Fatal("expected error for method not found, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", st.Code())
	}
}

// TestDiscover_EmptyMethodName validates that Discover returns InvalidArgument when
// called with an empty method name. This validates that the validation layer properly
// rejects malformed discover requests.
func TestDiscover_EmptyMethodName(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: ""}
	_, err := server.Discover(t.Context(), req)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty method name, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// TestDiscover_InvalidMethodNames validates that Discover returns InvalidArgument for
// various invalid method name formats. Method name validation is comprehensive in the
// FuzzRegister_MethodNames test; this test covers key invalid cases for Discover.
func TestDiscover_InvalidMethodNames(t *testing.T) {
	tests := []struct {
		name       string
		methodName string
	}{
		{"not qualified", "GetPrincipal"},
		{"no leading slash", "service.Service/Method"},
		{"trailing slash", "/service.Service/Method/"},
		{"consecutive slashes", "/service.Service//Method"},
		{"whitespace", "/service.Service/ Method"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			meter := meter.New()
			store := mock.NewStore()
			server := NewGRPCDServer(log, store, meter)

			// Execute
			req := &grpcd.DiscoverRequest{MethodName: tt.methodName}
			_, err := server.Discover(t.Context(), req)

			// Assert
			if err == nil {
				t.Fatalf("expected error for invalid method name %q, got nil", tt.methodName)
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatal("expected gRPC status error")
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument, got %v", st.Code())
			}
		})
	}
}

// TestDiscover_StorageError validates that Discover returns Internal error when the
// storage backend fails during lookup. This ensures storage failures are properly
// surfaced to the caller rather than being misinterpreted as "method not found".
func TestDiscover_StorageError(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Inject storage error BEFORE calling Discover
	store.SetGetMethodAddressError(errors.New("storage unavailable"))

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: "/test.Service/Method"}
	_, err := server.Discover(t.Context(), req)

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

// TestDiscover_ExpiredRegistration validates that Discover returns NotFound when
// attempting to discover a method whose registration has expired. This ensures TTLs
// are properly enforced and expired registrations are indistinguishable from
// never-registered methods.
func TestDiscover_ExpiredRegistration(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Seed data: Register a method with very short TTL
	methodName := "/test.Service/Method"
	serviceAddr := "192.168.1.100:50054"
	err := store.SetMethodAddress(t.Context(), methodName, serviceAddr, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to seed method: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: methodName}
	_, err = server.Discover(t.Context(), req)

	// Assert - should return NotFound, not the expired address
	if err == nil {
		t.Fatal("expected error for expired registration, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound for expired registration, got %v", st.Code())
	}
}

// TestDiscover_MultipleServicesLastWins validates that when multiple services register
// the same method, Discover returns the address from the most recent registration.
// This implements last-writer-wins semantics consistent with Register behavior.
func TestDiscover_MultipleServicesLastWins(t *testing.T) {
	// Setup
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := meter.New()
	store := mock.NewStore()
	server := NewGRPCDServer(log, store, meter)

	// Seed data: Register same method from two addresses
	methodName := "/test.Service/Method"
	addr1 := "192.168.1.100:50054"
	addr2 := "192.168.1.200:50054"

	// First registration
	err := store.SetMethodAddress(t.Context(), methodName, addr1, 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to seed first registration: %v", err)
	}

	// Second registration overwrites first
	err = store.SetMethodAddress(t.Context(), methodName, addr2, 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to seed second registration: %v", err)
	}

	// Execute
	req := &grpcd.DiscoverRequest{MethodName: methodName}
	resp, err := server.Discover(t.Context(), req)

	// Assert - should return second address
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Address != addr2 {
		t.Errorf("expected last registered address %s, got %s", addr2, resp.Address)
	}
}
