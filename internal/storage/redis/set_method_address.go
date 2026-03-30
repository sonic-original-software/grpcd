package redis

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// SetMethodAddress stores a method → address mapping with TTL
func (r *Store) SetMethodAddress(ctx context.Context, method, address string, ttl time.Duration) error {
	methodKey := storage.MethodKey(method)
	addressKey := storage.AddressKey(address)

	// Use pipeline for atomicity
	pipe := r.client.Pipeline()

	// Set method → address with TTL
	pipe.SetEx(ctx, methodKey, address, ttl)

	// Add method to address's reverse index (no TTL on set)
	pipe.SAdd(ctx, addressKey, method)

	_, err := pipe.Exec(ctx)
	return err
}
