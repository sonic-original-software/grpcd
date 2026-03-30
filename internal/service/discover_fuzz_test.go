package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	grpcd "git.sonicoriginal.software/grpcd-protos"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"

	"git.sonicoriginal.software/grpc-testing/mocks/meter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FuzzDiscover_MethodNames validates that Discover properly handles all possible method
// name inputs. This is THE most critical fuzz test for the grpcd service - if Discover
// fails or returns incorrect results, the entire Cumulus service mesh breaks down. Every
// method name variation must be tested to ensure only valid, registered methods return
// addresses and invalid names return appropriate errors.
func FuzzDiscover_MethodNames(f *testing.F) {
	// Seed corpus with valid and invalid cases
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

		// Determine if method name is valid
		isValid := isValidMethodName(methodName)

		// If valid, seed it in storage
		expectedAddr := "192.168.1.100:50054"
		if isValid {
			err := store.SetMethodAddress(t.Context(), methodName, expectedAddr, 10*time.Minute)
			if err != nil {
				t.Fatalf("failed to seed valid method: %v", err)
			}
		}

		// Execute Discover
		req := &grpcd.DiscoverRequest{MethodName: methodName}
		resp, err := server.Discover(t.Context(), req)

		// Validate behavior
		if isValid {
			// Valid method names should return the registered address
			if err != nil {
				t.Errorf("expected success for valid method name %q, got error: %v", methodName, err)
			}
			if resp == nil {
				t.Errorf("expected response for valid method name %q, got nil", methodName)
			} else if resp.Address != expectedAddr {
				t.Errorf("expected address %s for method %q, got %s", expectedAddr, methodName, resp.Address)
			}
		} else {
			// Invalid method names should return InvalidArgument
			if err == nil {
				t.Errorf("expected error for invalid method name %q, got success", methodName)
				return
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Errorf("expected gRPC status error for invalid method name %q", methodName)
				return
			}
			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument for invalid method name %q, got %v", methodName, st.Code())
			}
		}
	})
}
