//revive:disable:package-comments
package redis

import (
	"github.com/redis/go-redis/v9"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// Store implements Store using Redis as the backend
type Store struct {
	client     *redis.Client
	additions  *storage.Additions
	conditions *storage.Conditions
}

// NewRedisStore creates a new Redis store
// Performs pure construction with no network I/O
func NewRedisStore(addr string) *Store {
	client := redis.NewClient(&redis.Options{Addr: addr})

	return &Store{
		client:     client,
		additions:  storage.NewAdditions(),
		conditions: storage.NewConditions(),
	}
}

// Latest answers with the most recent addition announced to this instance.
func (r *Store) Latest() *storage.Addition {
	return r.additions.Latest()
}

// Condition answers with the store's reachability.
func (r *Store) Condition() *storage.Condition {
	return r.conditions.Current()
}

// Close closes the Redis connection
func (r *Store) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Name of the store
func (*Store) Name() string {
	return "redis"
}

// Address of the store
func (r *Store) Address() string {
	return r.client.Options().Addr
}
