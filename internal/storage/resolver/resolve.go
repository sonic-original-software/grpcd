//revive:disable:package-comments
package resolver

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"
	"git.sonicoriginal.software/grpcd/internal/storage/redis"
)

// Resolve a storage Store by the storage backend and address env variables.
//
// An unreachable backend is returned as an error rather than ended here, so the
// caller's shutdown still runs and the log explaining the failure is exported.
func Resolve(ctx context.Context, log *slog.Logger) (storage.Store, error) {
	backend := os.Getenv(storage.EnvStorageBackend)
	address := os.Getenv(storage.EnvStorageAddress)

	switch backend {
	case "redis":
		store := redis.NewRedisStore(address)

		if err := store.Ping(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", storage.ErrStorageNotReachable, err)
		}

		if err := store.Listen(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", storage.ErrStorageNotReachable, err)
		}

		log.InfoContext(ctx, "Using redis storage", "address", address)

		return store, nil
	case "":
		log.InfoContext(ctx, "Using volatile storage")
		return mock.NewStore(), nil
	default:
		return nil, fmt.Errorf("%w: unknown backend %q", storage.ErrStorageNotConfigured, backend)
	}
}
