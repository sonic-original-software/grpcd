package service

import (
	"errors"
	"log/slog"
	"testing"

	"git.sonicoriginal.software/grpc-testing/mocks/meter"

	"github.com/grpcd/server/internal/storage/mock"
)

// newStorageCheckServer builds a server over a mock store whose Ping fails with
// pingErr, or succeeds when pingErr is nil.
func newStorageCheckServer(pingErr error) *GRPCDServer {
	store := mock.NewStore()
	store.SetPingError(pingErr)

	return NewGRPCDServer(slog.New(slog.DiscardHandler), store, meter.New(), testAnchor)
}

func TestStorageCheck(t *testing.T) {
	t.Run("reports the store when it answers", func(t *testing.T) {
		server := newStorageCheckServer(nil)

		got, err := server.StorageCheck(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.State != "" {
			t.Errorf("state = %q, want empty", got.State)
		}
		if got.Details["name"] != "mock" {
			t.Errorf("details name = %q, want %q", got.Details["name"], "mock")
		}
		if got.LastChecked == 0 {
			t.Error("last checked was not recorded")
		}
	})

	t.Run("reports the failure as the store's state", func(t *testing.T) {
		wantErr := errors.New("storage unreachable")
		server := newStorageCheckServer(wantErr)

		got, err := server.StorageCheck(t.Context())
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if got.State != wantErr.Error() {
			t.Errorf("state = %q, want %q", got.State, wantErr.Error())
		}
		if got.Details["name"] != "mock" {
			t.Errorf("details name = %q, want %q", got.Details["name"], "mock")
		}
	})
}
