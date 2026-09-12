package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcd "git.sonicoriginal.software/grpcd-protos"
)

// held runs Register on its own goroutine and answers with a channel carrying
// its return value, because the handler does not return until the stream ends.
func held(
	server *GRPCDServer, request *grpcd.RegisterRequest, stream *registerStream,
) <-chan error {
	returned := make(chan error, 1)

	go func() { returned <- server.Register(request, stream) }()

	return returned
}

// await fails the test if signal does not fire, turning a handler that never
// gets there into a failure rather than a hang.
func await[T any](t *testing.T, signal <-chan T, message string) T {
	t.Helper()

	select {
	case value := <-signal:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal(message)
	}

	var zero T

	return zero
}

func TestRegister(t *testing.T) {
	methods := []string{
		"/package.Service/Method",
		"/package.Service/Other",
	}

	t.Run("holds the rows for as long as the stream", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)

		await(t, stream.acknowledged(), "registration was never acknowledged")

		// The address pairs the IP off the connection with the port the caller
		// reported, rather than the ephemeral port it dialed from.
		for _, method := range methods {
			if got := store.Addresses(method); !slices.Equal(got, []string{"192.168.1.100:50054"}) {
				t.Errorf("method %s holds %v", method, got)
			}
		}

		if got := store.Anchor("192.168.1.100:50054"); got != testAnchor {
			t.Errorf("expected anchor %q, got %q", testAnchor, got)
		}

		disconnect()

		if err := await(t, returned, "handler did not return when the stream ended"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		for _, method := range methods {
			if got := store.Addresses(method); len(got) != 0 {
				t.Errorf("method %s still holds %v after the stream ended", method, got)
			}
		}
	})

	t.Run("leaves other addresses serving the method", func(t *testing.T) {
		server, store := newServer()

		first, disconnectFirst := context.WithCancel(peerContext(t.Context(), "10.0.0.1:41234"))
		defer disconnectFirst()

		second, disconnectSecond := context.WithCancel(peerContext(t.Context(), "10.0.0.2:41234"))
		defer disconnectSecond()

		firstStream, secondStream := newRegisterStream(first), newRegisterStream(second)

		request := &grpcd.RegisterRequest{Methods: methods[:1], Port: 50054}

		held(server, request, firstStream)
		await(t, firstStream.acknowledged(), "first registration was never acknowledged")

		returned := held(server, request, secondStream)
		await(t, secondStream.acknowledged(), "second registration was never acknowledged")

		disconnectSecond()
		await(t, returned, "second handler did not return")

		if got := store.Addresses(methods[0]); !slices.Equal(got, []string{"10.0.0.1:50054"}) {
			t.Errorf("expected the first address to survive, got %v", got)
		}
	})

	t.Run("refuses a request with no methods", func(t *testing.T) {
		server, _ := newServer()

		ctx := peerContext(t.Context(), "192.168.1.100:41234")

		err := server.Register(&grpcd.RegisterRequest{Port: 50054}, newRegisterStream(ctx))

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("refuses a request with no port", func(t *testing.T) {
		server, _ := newServer()

		ctx := peerContext(t.Context(), "192.168.1.100:41234")

		err := server.Register(&grpcd.RegisterRequest{Methods: methods}, newRegisterStream(ctx))

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("refuses a port outside the range", func(t *testing.T) {
		server, _ := newServer()

		ctx := peerContext(t.Context(), "192.168.1.100:41234")

		err := server.Register(
			&grpcd.RegisterRequest{Methods: methods, Port: 70000}, newRegisterStream(ctx),
		)

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("refuses a connection carrying no peer", func(t *testing.T) {
		server, _ := newServer()

		err := server.Register(
			&grpcd.RegisterRequest{Methods: methods, Port: 50054},
			newRegisterStream(t.Context()),
		)

		assertCode(t, err, codes.Internal)
	})

	t.Run("composes an address from a peer with no port", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "/tmp/grpcd.sock"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		held(server, &grpcd.RegisterRequest{Methods: methods[:1], Port: 50054}, stream)
		await(t, stream.acknowledged(), "registration was never acknowledged")

		if got := store.Addresses(methods[0]); len(got) != 1 {
			t.Fatalf("expected one address, got %v", got)
		}
	})

	t.Run("refuses when the rows cannot be written", func(t *testing.T) {
		server, store := newServer()

		store.SetAddError(errors.New("storage unavailable"))

		ctx := peerContext(t.Context(), "192.168.1.100:41234")

		err := server.Register(
			&grpcd.RegisterRequest{Methods: methods, Port: 50054}, newRegisterStream(ctx),
		)

		assertCode(t, err, codes.Internal)
	})

	t.Run("removes the rows when the acknowledgement cannot be sent", func(t *testing.T) {
		server, store := newServer()

		ctx := peerContext(t.Context(), "192.168.1.100:41234")

		stream := newRegisterStream(ctx)
		stream.sendErr = errors.New("broken transport")

		err := server.Register(
			&grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream,
		)

		if err == nil {
			t.Fatal("expected the send failure to be returned")
		}

		if got := store.Addresses(methods[0]); len(got) != 0 {
			t.Errorf("expected the rows to be removed, got %v", got)
		}
	})

	t.Run("returns when the rows cannot be removed", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)
		await(t, stream.acknowledged(), "registration was never acknowledged")

		store.SetRemoveError(errors.New("storage unavailable"))

		disconnect()

		if err := await(t, returned, "handler did not return"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("holds the registration through a lost store and writes it once the store returns", func(t *testing.T) {
		server, store := newServer()

		store.SetAddError(errors.New("storage unavailable"))
		store.Lose()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		failed := store.Failed()
		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)

		// The write failed against a lost store, so the handler is waiting
		// rather than refusing.
		await(t, failed, "handler never tried to write")

		if got := stream.sends(); got != 0 {
			t.Fatalf("expected no acknowledgement while the store is lost, got %d", got)
		}

		store.SetAddError(nil)
		store.Recover()

		await(t, stream.acknowledged(), "registration was never acknowledged after the store returned")

		if got := store.Addresses(methods[0]); !slices.Equal(got, []string{"192.168.1.100:50054"}) {
			t.Errorf("expected the rows to be written, got %v", got)
		}

		disconnect()
		await(t, returned, "handler did not return")
	})

	t.Run("returns when the caller leaves while the store is lost", func(t *testing.T) {
		server, store := newServer()

		store.SetAddError(errors.New("storage unavailable"))
		store.Lose()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		failed := store.Failed()
		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, newRegisterStream(ctx))

		await(t, failed, "handler never tried to write")

		disconnect()

		if err := await(t, returned, "handler did not return"); err == nil {
			t.Fatal("expected the cancellation to be returned")
		}
	})

	t.Run("rewrites the rows when the store returns while holding", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)
		await(t, stream.acknowledged(), "registration was never acknowledged")

		// Losing the store while holding is only something to wait out: the
		// handler looks, sees it lost, and sleeps again without writing.
		conditioned := store.Conditioned()
		store.Lose()
		await(t, conditioned, "handler did not look at the store after it was lost")

		// The store coming back may have come back empty, so the rows are
		// written again from the request the handler holds.
		added := store.Added()
		store.Recover()
		await(t, added, "handler did not rewrite the rows after the store returned")

		if got := store.Adds(); got != 2 {
			t.Errorf("expected the rows to be written twice, got %d", got)
		}

		disconnect()
		await(t, returned, "handler did not return")
	})

	t.Run("keeps holding when the rewrite fails", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)
		await(t, stream.acknowledged(), "registration was never acknowledged")

		store.SetAddError(errors.New("storage unavailable"))

		failed := store.Failed()
		store.Recover()
		await(t, failed, "handler did not try to rewrite the rows")

		// A later recovery is tried again.
		store.SetAddError(nil)

		added := store.Added()
		store.Recover()
		await(t, added, "handler did not rewrite the rows on the next recovery")

		disconnect()
		await(t, returned, "handler did not return")
	})

	t.Run("acknowledges once", func(t *testing.T) {
		server, _ := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "192.168.1.100:41234"))
		defer disconnect()

		stream := newRegisterStream(ctx)

		returned := held(server, &grpcd.RegisterRequest{Methods: methods, Port: 50054}, stream)
		await(t, stream.acknowledged(), "registration was never acknowledged")

		disconnect()
		await(t, returned, "handler did not return")

		if got := stream.sends(); got != 1 {
			t.Errorf("expected one acknowledgement, got %d", got)
		}
	})
}

// assertCode fails the test unless err is a gRPC status carrying want.
func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected %v, got no error", want)
	}

	got, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status, got %v", err)
	}

	if got.Code() != want {
		t.Errorf("expected %v, got %v", want, got.Code())
	}
}
