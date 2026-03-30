package service

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	grpcd "git.sonicoriginal.software/grpcd-protos"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-testing/mocks/addr"
	"git.sonicoriginal.software/grpc-testing/mocks/meter"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// FuzzRegister_MethodNames validates that Register properly handles all possible method
// name inputs, rejecting invalid names with InvalidArgument and accepting valid ones.
// This is critical for ensuring malformed method names cannot be registered.
func FuzzRegister_MethodNames(f *testing.F) {
	// Seed corpus with known valid and invalid cases
	f.Add("/Service/Method")
	f.Add("/package.service.Service/Method")
	f.Add("/a.b.c.d.Service/Method")
	f.Add("")
	f.Add("NoQualification")
	f.Add("/Service//Method")
	f.Add("Service/Method")
	f.Add("/Service/Method/")
	f.Add(" /Service/Method")
	f.Add("/Service/Method ")
	f.Add("/Service/ Method")
	f.Add("/Service")

	f.Fuzz(func(t *testing.T, methodName string) {
		// Setup
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		meter := meter.New()
		store := mock.NewStore()
		server := NewGRPCDServer(log, store, meter)

		// Create context with peer info
		addr := addr.New("192.168.1.100:50054")
		ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

		// Test data
		req := &grpcd.RegisterRequest{Methods: []string{methodName}}

		// Execute
		_, err := server.Register(ctx, req)

		// Determine if the method name should be valid
		isValid := isValidMethodName(methodName)

		if isValid {
			// Valid method names should succeed
			if err != nil {
				t.Errorf("expected valid method name %q to succeed, got error: %v", methodName, err)
			}
		} else {
			// Invalid method names should fail with InvalidArgument
			if err == nil {
				t.Errorf("expected invalid method name %q to fail, got success", methodName)
				return
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Errorf("expected gRPC status error for invalid method name %q, got: %v", methodName, err)
				return
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument for invalid method name %q, got %v", methodName, st.Code())
			}
		}
	})
}

// FuzzRegister_MethodCounts validates that Register properly handles varying numbers of
// methods in a single registration request, from zero methods (should fail validation)
// to large batches. This ensures Register scales correctly and validates array size constraints.
func FuzzRegister_MethodCounts(f *testing.F) {
	// Seed corpus with interesting method counts
	f.Add(0)    // Empty array - should fail
	f.Add(1)    // Single method
	f.Add(3)    // Small batch
	f.Add(10)   // Medium batch
	f.Add(100)  // Large batch
	f.Add(1000) // Very large batch

	f.Fuzz(func(t *testing.T, methodCount int) {
		// Skip negative and unreasonably large counts
		if methodCount < 0 || methodCount > 10000 {
			t.Skip()
		}

		// Setup
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		meter := meter.New()
		store := mock.NewStore()
		server := NewGRPCDServer(log, store, meter)

		// Create context with peer info
		addr := addr.New("192.168.1.100:50054")
		ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})

		// Generate N valid method names in gRPC format
		methods := make([]string, methodCount)
		for i := 0; i < methodCount; i++ {
			methods[i] = "/Service/Method" + intToString(i)
		}

		// Execute
		req := &grpcd.RegisterRequest{Methods: methods}
		_, err := server.Register(ctx, req)

		// Validate behavior based on method count
		if methodCount == 0 {
			// Should fail validation
			if err == nil {
				t.Error("expected error for zero methods, got nil")
			}
			st, ok := status.FromError(err)
			if ok && st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument for zero methods, got %v", st.Code())
			}
		} else {
			// Should succeed
			if err != nil {
				t.Errorf("expected success for %d methods, got error: %v", methodCount, err)
			}

			// Verify all methods were registered
			for _, method := range methods {
				storedAddr, err := store.GetMethodAddress(ctx, method)
				if err != nil {
					t.Errorf("method %s should be registered", method)
				} else if storedAddr != addr.String() {
					t.Errorf("method %s has wrong address: got %s, want %s", method, storedAddr, addr.String())
				}
			}
		}
	})
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var result strings.Builder
	for n > 0 {
		result.WriteByte(byte('0' + n%10))
		n /= 10
	}
	// Reverse
	s := result.String()
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
