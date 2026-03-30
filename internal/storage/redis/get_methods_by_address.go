package redis

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// GetMethodsByAddress retrieves all methods registered for an address
func (r *Store) GetMethodsByAddress(ctx context.Context, address string) ([]string, error) {
	addressKey := storage.AddressKey(address)

	methods, err := r.client.SMembers(ctx, addressKey).Result()
	if err != nil {
		return nil, err
	}

	return methods, nil
}
