package redis

import "context"

// Ping checks if Redis is reachable
func (r *Store) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
