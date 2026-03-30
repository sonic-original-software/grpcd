package redis

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"

	"github.com/redis/go-redis/v9"
)

// DeleteMethodAddress deletes a method → address mapping
func (r *Store) DeleteMethodAddress(ctx context.Context, method string) error {
	methodKey := storage.MethodKey(method)

	// First get the address to update reverse index
	address, err := r.client.Get(ctx, methodKey).Result()
	if err == redis.Nil {
		// Method doesn't exist, nothing to delete
		return nil
	}
	if err != nil {
		return err
	}

	addressKey := storage.AddressKey(address)

	// Use pipeline
	pipe := r.client.Pipeline()

	// Delete method key
	pipe.Del(ctx, methodKey)

	// Remove method from address's reverse index
	pipe.SRem(ctx, addressKey, method)

	_, err = pipe.Exec(ctx)
	return err
}
