package redis

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"

	"github.com/redis/go-redis/v9"
)

// GetMethodAddress retrieves the address for a method
func (r *Store) GetMethodAddress(ctx context.Context, method string) (string, error) {
	methodKey := storage.MethodKey(method)

	address, err := r.client.Get(ctx, methodKey).Result()
	if err == redis.Nil {
		return "", storage.ErrMethodNotFound
	}
	if err != nil {
		return "", err
	}

	return address, nil
}
