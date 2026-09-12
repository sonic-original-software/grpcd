package service

import (
	"context"
	"strconv"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcd "github.com/grpcd/protos"
)

// endedStream is a registration stream whose caller is already gone, so the
// handler runs its whole path — write, acknowledge, remove — without blocking.
func endedStream(t *testing.T, address string) *registerStream {
	t.Helper()

	ctx, disconnect := context.WithCancel(peerContext(t.Context(), address))
	disconnect()

	return newRegisterStream(ctx)
}

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
		server, _ := newServer()

		stream := endedStream(t, "192.168.1.100:41234")

		err := server.Register(
			&grpcd.RegisterRequest{Methods: []string{methodName}, Port: 50054}, stream,
		)

		isValid := isValidMethodName(methodName)

		if isValid {
			// Valid method names should succeed
			if err != nil {
				t.Errorf("expected valid method name %q to succeed, got error: %v", methodName, err)

				return
			}

			if stream.sends() != 1 {
				t.Errorf("expected method name %q to be acknowledged", methodName)
			}

			return
		}

		// Invalid method names should fail with InvalidArgument
		if err == nil {
			t.Errorf("expected invalid method name %q to fail, got success", methodName)

			return
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Errorf(
				"expected gRPC status error for invalid method name %q, got: %v", methodName, err,
			)

			return
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf(
				"expected InvalidArgument for invalid method name %q, got %v",
				methodName, st.Code(),
			)
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

		server, store := newServer()

		// Generate N valid method names in gRPC format
		methods := make([]string, methodCount)
		for i := range methods {
			methods[i] = "/Service/Method" + strconv.Itoa(i)
		}

		// A stream that is still held, so the rows are there to assert on.
		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)

		if methodCount == 0 {
			// Should fail validation
			err := await(t, returned, "handler did not return for zero methods")
			if err == nil {
				t.Fatal("expected error for zero methods, got nil")
			}

			st, ok := status.FromError(err)
			if ok && st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument for zero methods, got %v", st.Code())
			}

			return
		}

		await(t, stream.acknowledged(), "registration was never acknowledged")

		// Verify all methods were registered against the composed address
		for _, method := range methods {
			addresses := store.Addresses(method)

			if len(addresses) != 1 || addresses[0] != "192.168.1.100:50054" {
				t.Errorf("method %s holds %v", method, addresses)

				break
			}
		}

		disconnect()

		if err := await(t, returned, "handler did not return"); err != nil {
			t.Errorf("expected success for %d methods, got error: %v", methodCount, err)
		}
	})
}
