package redis

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// DeleteMethodsByAddress deletes all methods for an address
func (r *Store) DeleteMethodsByAddress(ctx context.Context, address string) error {
	addressKey := storage.AddressKey(address)

	// Get all methods first
	methods, err := r.client.SMembers(ctx, addressKey).Result()
	if err != nil {
		return err
	}

	if len(methods) == 0 {
		// No methods to delete
		return nil
	}

	// Use pipeline to delete all method keys and the address set
	pipe := r.client.Pipeline()

	// Delete all method keys
	for _, method := range methods {
		methodKey := storage.MethodKey(method)
		pipe.Del(ctx, methodKey)
	}

	// Delete the address set itself
	pipe.Del(ctx, addressKey)

	_, err = pipe.Exec(ctx)
	return err
}
