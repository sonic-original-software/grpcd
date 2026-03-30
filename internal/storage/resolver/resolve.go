//revive:disable:package-comments
package resolver

import (
	"context"
	"log/slog"
	"os"

	"git.sonicoriginal.software/grpcd/internal/storage"
	"git.sonicoriginal.software/grpcd/internal/storage/mock"
	"git.sonicoriginal.software/grpcd/internal/storage/redis"
)

// Resolve a storage Store by the storage backend and address env variables
func Resolve(ctx context.Context, log *slog.Logger) (storage.Store, error) {
	backend := os.Getenv(storage.EnvStorageBackend)
	address := os.Getenv(storage.EnvStorageAddress)

	var store storage.Store

	if backend == "redis" {
		store = redis.NewRedisStore(address)
		if err := store.Ping(ctx); err != nil {
			log.ErrorContext(ctx, storage.ErrStorageNotReachable.Error())
			os.Exit(1)
		}
		log.Info("Using redis storage", "address", address)
	} else if backend == "" || address == "" {
		log.Info("Using volatile storage")
		store = mock.NewStore()
	} else {
		return nil, storage.ErrStorageNotConfigured
	}

	return store, nil
}
