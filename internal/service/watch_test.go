package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	grpcd "git.sonicoriginal.software/grpcd-protos"
)

// watchStream stands in for a held Watch stream and records what it was told.
type watchStream struct {
	grpc.ServerStream

	ctx context.Context

	mu      sync.Mutex
	sent    []string
	sendErr error

	// told carries each address as it is sent, so a test can wait for one.
	told chan string
}

func newWatchStream(ctx context.Context) *watchStream {
	return &watchStream{ctx: ctx, told: make(chan string, 8)}
}

func (s *watchStream) Context() context.Context { return s.ctx }

func (s *watchStream) Send(response *grpcd.WatchResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendErr != nil {
		return s.sendErr
	}

	s.sent = append(s.sent, response.Address)
	s.told <- response.Address

	return nil
}

func (s *watchStream) sends() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.sent...)
}

// watching runs Watch on its own goroutine, because it holds the stream until
// the caller leaves.
func watching(server *GRPCDServer, req *grpcd.WatchRequest, stream *watchStream) <-chan error {
	returned := make(chan error, 1)

	go func() { returned <- server.Watch(req, stream) }()

	return returned
}

// holding is a request holding address for method.
func holding(address string) *grpcd.WatchRequest {
	return &grpcd.WatchRequest{MethodName: method, Address: address}
}

// always is a draw every holder wins.
func always(int64) bool { return true }

func TestWatch(t *testing.T) {
	t.Run("refuses an invalid method name", func(t *testing.T) {
		server, _ := newServer()

		err := server.Watch(
			&grpcd.WatchRequest{MethodName: "not-a-method", Address: "10.0.0.1:50054"},
			newWatchStream(t.Context()),
		)

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("refuses an empty address", func(t *testing.T) {
		server, _ := newServer()

		err := server.Watch(holding(""), newWatchStream(t.Context()))

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("returns when the caller goes away", func(t *testing.T) {
		server, _ := newServer()

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		returned := watching(server, holding("10.0.0.1:50054"), newWatchStream(ctx))

		leave()

		if err := await(t, returned, "handler did not return"); err == nil {
			t.Fatal("expected the cancellation to be returned")
		}
	})

	t.Run("tells the holder about a new address when the draw wins", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		stream := newWatchStream(ctx)
		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), stream)

		// Registered only once the handler holds the announcement it sleeps
		// on, so the registration reaches it as a wake.
		await(t, loaded, "handler never began watching")

		if err := store.Add(ctx, "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		if got := await(t, stream.told, "holder was never told"); got != "10.0.0.2:50054" {
			t.Errorf("told %q, want 10.0.0.2:50054", got)
		}

		leave()
		await(t, returned, "handler did not return")
	})

	t.Run("says nothing about other methods or the address held", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		stream := newWatchStream(ctx)
		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), stream)

		await(t, loaded, "handler never began watching")

		// Each registration is made only once the handler has reloaded after
		// the previous one, so every wake is seen and judged on its own.
		for _, registration := range []struct{ address, method string }{
			{"10.0.0.9:50054", "/other.Service/Method"},
			{"10.0.0.1:50054", method},
		} {
			loaded = store.Loaded()

			if err := store.Add(ctx, registration.address, testAnchor, []string{registration.method}); err != nil {
				t.Fatalf("failed to register: %v", err)
			}

			await(t, loaded, "handler did not wake")
		}

		if err := store.Add(ctx, "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		await(t, stream.told, "holder was never told")

		leave()
		await(t, returned, "handler did not return")

		if got := stream.sends(); !slices.Equal(got, []string{"10.0.0.2:50054"}) {
			t.Errorf("told %v, want only 10.0.0.2:50054", got)
		}
	})

	t.Run("says nothing when the draw loses", func(t *testing.T) {
		server, store := newServer()

		// Wins only once three addresses serve the method, so the second
		// registration is told and the first is not.
		server.roll = func(n int64) bool { return n == 3 }

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		if err := store.Add(ctx, "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		stream := newWatchStream(ctx)
		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), stream)

		await(t, loaded, "handler never began watching")

		loaded = store.Loaded()

		if err := store.Add(ctx, "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		await(t, loaded, "handler did not wake")

		if err := store.Add(ctx, "10.0.0.3:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		await(t, stream.told, "holder was never told")

		leave()
		await(t, returned, "handler did not return")

		if got := stream.sends(); !slices.Equal(got, []string{"10.0.0.3:50054"}) {
			t.Errorf("told %v, want only 10.0.0.3:50054", got)
		}
	})

	t.Run("returns when the count fails", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		store.SetCountError(errors.New("storage unavailable"))

		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), newWatchStream(t.Context()))

		await(t, loaded, "handler never began watching")

		if err := store.Add(t.Context(), "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		assertCode(t, await(t, returned, "handler did not return"), codes.Internal)
	})

	t.Run("holds the watch through a lost store", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		store.SetCountError(errors.New("storage unavailable"))
		store.Lose()

		stream := newWatchStream(ctx)
		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), stream)

		await(t, loaded, "handler never began watching")

		failed := store.Failed()

		if err := store.Add(ctx, "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		await(t, failed, "handler never tried to count")

		// The addition that woke the handler is gone with the store; the next
		// one after recovery is told.
		store.SetCountError(nil)
		store.Recover()

		if err := store.Add(ctx, "10.0.0.3:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		if got := await(t, stream.told, "holder was never told"); got != "10.0.0.3:50054" {
			t.Errorf("told %q, want 10.0.0.3:50054", got)
		}

		leave()
		await(t, returned, "handler did not return")
	})

	t.Run("returns when the caller goes away while the store is lost", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		ctx, leave := context.WithCancel(t.Context())
		defer leave()

		store.SetCountError(errors.New("storage unavailable"))
		store.Lose()

		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), newWatchStream(ctx))

		await(t, loaded, "handler never began watching")

		failed := store.Failed()

		if err := store.Add(ctx, "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		await(t, failed, "handler never tried to count")

		leave()

		if err := await(t, returned, "handler did not return"); err == nil {
			t.Fatal("expected the cancellation to be returned")
		}
	})

	t.Run("returns when the holder cannot be told", func(t *testing.T) {
		server, store := newServer()
		server.roll = always

		stream := newWatchStream(t.Context())
		stream.sendErr = errors.New("broken transport")

		loaded := store.Loaded()
		returned := watching(server, holding("10.0.0.1:50054"), stream)

		await(t, loaded, "handler never began watching")

		if err := store.Add(t.Context(), "10.0.0.2:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to register: %v", err)
		}

		if err := await(t, returned, "handler did not return"); err == nil {
			t.Fatal("expected the send failure to be returned")
		}
	})
}
