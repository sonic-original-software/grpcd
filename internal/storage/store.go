//revive:disable:package-comments
package storage

import (
	"context"
	"errors"
	"time"
)

const (
	// EnvStorageBackend is the environment variable key for storage backend type
	EnvStorageBackend = "STORAGE_BACKEND"

	// EnvStorageAddress is the environment variable key for storage backend address
	EnvStorageAddress = "STORAGE_ADDRESS"

	// ErrCodeStorageNotConfigured is the code for storage that is not configured
	ErrCodeStorageNotConfigured = "STORAGE_NOT_CONFIGURED"
)

var (
	// ErrMethodNotFound is returned when a method mapping is not found
	ErrMethodNotFound = errors.New("method not found")

	// ErrStorageNotConfigured is returned when storage environment variables are not set
	ErrStorageNotConfigured = errors.New("storage not configured")

	// ErrStorageNotReachable is returned when storage backend is not reachable
	ErrStorageNotReachable = errors.New("storage not reachable")
)

// Store defines the interface for grpcd service storage operations
// Stores method → address mappings with TTL-based expiry
type Store interface {
	// SetMethodAddress stores a method → address mapping with TTL
	// If the method already exists, it overwrites the address and resets TTL
	SetMethodAddress(ctx context.Context, method, address string, ttl time.Duration) error

	// GetMethodAddress retrieves the address for a method
	// Returns ErrMethodNotFound if the method is not registered
	GetMethodAddress(ctx context.Context, method string) (string, error)

	// DeleteMethodAddress removes a specific method → address mapping
	DeleteMethodAddress(ctx context.Context, method string) error

	// GetMethodsByAddress retrieves all methods registered by a specific address
	// Used for bulk operations during deregistration
	GetMethodsByAddress(ctx context.Context, address string) ([]string, error)

	// DeleteMethodsByAddress removes all methods registered by a specific address
	// Used during deregistration to clean up all methods for an instance
	DeleteMethodsByAddress(ctx context.Context, address string) error

	// Ping checks storage connectivity
	Ping(ctx context.Context) error

	// Name of the storage implementation
	Name() string

	// Address of the storage if applicable
	Address() string
}
