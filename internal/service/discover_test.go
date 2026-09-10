package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"google.golang.org/grpc/codes"

	grpcd "git.sonicoriginal.software/grpcd-protos"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

const method = "/principal.pb.PrincipalService/GetPrincipal"

func TestDiscover(t *testing.T) {
	t.Run("offers a registered address", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		stream := satisfiedAfter(t.Context(), method)

		if err := server.Discover(stream); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if got := stream.candidates(); !slices.Equal(got, []string{"10.0.0.1:50054"}) {
			t.Errorf("expected one candidate, got %v", got)
		}
	})

	t.Run("offers the next address after one is reported dead", func(t *testing.T) {
		server, store := newServer()

		for _, address := range []string{"10.0.0.1:50054", "10.0.0.2:50054"} {
			if err := store.Add(t.Context(), address, testAnchor, []string{method}); err != nil {
				t.Fatalf("failed to seed: %v", err)
			}
		}

		stream := satisfiedAfter(t.Context(), method, "10.0.0.1:50054")

		if err := server.Discover(stream); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		want := []string{"10.0.0.1:50054", "10.0.0.2:50054"}
		if got := stream.candidates(); !slices.Equal(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}

		if got := store.Addresses(method); !slices.Equal(got, []string{"10.0.0.2:50054"}) {
			t.Errorf("expected the dead address to be removed, got %v", got)
		}
	})

	t.Run("reinstates a removal it is told about", func(t *testing.T) {
		server, store := newServer()

		ctx, disconnect := context.WithCancel(peerContext(t.Context(), "10.0.0.1:41234"))
		defer disconnect()

		registration := newRegisterStream(ctx)

		held(server, &grpcd.RegisterRequest{Methods: []string{method}, Port: 50054}, registration)
		await(t, registration.acknowledged(), "registration was never acknowledged")

		removals, err := store.Watch(t.Context(), testAnchor)
		if err != nil {
			t.Fatalf("failed to watch: %v", err)
		}

		// A client that cannot reach the address reports it, even though the
		// service is up and holding its stream.
		go func() {
			_ = server.Discover(satisfiedAfter(t.Context(), method, "10.0.0.1:50054"))
		}()

		removal := await(t, removals, "the anchor was never told about the removal")

		if removal.Method != method || removal.Address != "10.0.0.1:50054" {
			t.Fatalf("expected the removed row to be named, got %+v", removal)
		}

		server.Reinstate(t.Context(), removal)

		if got := store.Addresses(method); !slices.Equal(got, []string{"10.0.0.1:50054"}) {
			t.Errorf("expected the address to be written back, got %v", got)
		}
	})

	t.Run("reports when reinstating fails", func(t *testing.T) {
		server, store := newServer()

		store.SetAddError(errors.New("storage unavailable"))

		server.Reinstate(t.Context(), storage.Removal{Method: method, Address: "10.0.0.1:50054"})

		if got := store.Addresses(method); len(got) != 0 {
			t.Errorf("expected nothing to be written, got %v", got)
		}
	})

	t.Run("refuses a method that is not registered", func(t *testing.T) {
		server, _ := newServer()

		err := server.Discover(satisfiedAfter(t.Context(), method))

		assertCode(t, err, codes.NotFound)
	})

	t.Run("refuses a method whose addresses are all reported dead", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		err := server.Discover(asks(t.Context(), method, "10.0.0.1:50054"))

		assertCode(t, err, codes.NotFound)

		if got := store.Addresses(method); len(got) != 0 {
			t.Errorf("expected the dead address to be removed, got %v", got)
		}
	})

	t.Run("refuses an invalid method name", func(t *testing.T) {
		server, _ := newServer()

		err := server.Discover(satisfiedAfter(t.Context(), "not-a-method"))

		assertCode(t, err, codes.InvalidArgument)
	})

	t.Run("returns when the request cannot be read", func(t *testing.T) {
		server, _ := newServer()

		stream := &discoverStream{
			ctx:      t.Context(),
			requests: []recvResult{{err: errors.New("broken transport")}},
		}

		if err := server.Discover(stream); err == nil {
			t.Fatal("expected the receive failure to be returned")
		}
	})

	t.Run("returns when the store cannot be read", func(t *testing.T) {
		server, store := newServer()

		store.SetAddressesForError(errors.New("storage unavailable"))

		err := server.Discover(satisfiedAfter(t.Context(), method))

		assertCode(t, err, codes.Internal)
	})

	t.Run("returns when a candidate cannot be sent", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		stream := satisfiedAfter(t.Context(), method)
		stream.sendErr = errors.New("broken transport")

		if err := server.Discover(stream); err == nil {
			t.Fatal("expected the send failure to be returned")
		}
	})

	t.Run("returns when the report cannot be read", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		stream := asks(t.Context(), method)
		stream.requests = append(stream.requests, recvResult{err: errors.New("broken transport")})

		if err := server.Discover(stream); err == nil {
			t.Fatal("expected the receive failure to be returned")
		}
	})

	t.Run("ignores a report naming no address", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		stream := asks(t.Context(), method)
		stream.requests = append(stream.requests, recvResult{request: &grpcd.DiscoverRequest{}})

		err := server.Discover(stream)

		assertCode(t, err, codes.NotFound)

		if got := store.Addresses(method); !slices.Equal(got, []string{"10.0.0.1:50054"}) {
			t.Errorf("expected the address to survive, got %v", got)
		}
	})

	t.Run("keeps going when a removal fails", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		store.SetRemoveFromMethodError(errors.New("storage unavailable"))

		err := server.Discover(asks(t.Context(), method, "10.0.0.1:50054"))

		assertCode(t, err, codes.NotFound)
	})

	t.Run("keeps going when the anchor cannot be told", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", testAnchor, []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		store.SetNotifyError(errors.New("storage unavailable"))

		err := server.Discover(asks(t.Context(), method, "10.0.0.1:50054"))

		assertCode(t, err, codes.NotFound)
	})

	t.Run("tells nobody about an address with no anchor", func(t *testing.T) {
		server, store := newServer()

		if err := store.Add(t.Context(), "10.0.0.1:50054", "", []string{method}); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		err := server.Discover(asks(t.Context(), method, "10.0.0.1:50054"))

		assertCode(t, err, codes.NotFound)
	})
}
