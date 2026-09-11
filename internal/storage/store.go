//revive:disable:package-comments
package storage

import (
	"context"
	"errors"
	"iter"
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
	// ErrMethodNotFound is returned when a method has no registered addresses
	ErrMethodNotFound = errors.New("method not found")

	// ErrStorageNotConfigured is returned when storage environment variables are not set
	ErrStorageNotConfigured = errors.New("storage not configured")

	// ErrStorageNotReachable is returned when storage backend is not reachable
	ErrStorageNotReachable = errors.New("storage not reachable")
)

// Removal names a row that was taken out. It carries the method as well as the
// address so whoever anchored it can write back exactly what was removed
// without holding a record of what it registered.
type Removal struct {
	Method  string
	Address string
}

// Store holds which addresses serve which methods, and which instance anchors
// each address.
//
// Nothing it stores expires. A row is removed when the instance holding that
// address's registration stream sees the stream end, or when a client reports
// it could not reach the address.
type Store interface {
	// Add records that address serves each of methods, anchored to the instance
	// identified by anchor.
	//
	// Registering an address that is already present leaves the set as it was,
	// so a re-registration after a lost instance is the same write barring the
	// anchor changing.
	Add(ctx context.Context, address, anchor string, methods []string) error

	// Remove takes address out of each of methods and forgets its anchor.
	//
	// The caller is the instance whose registration stream just ended, and it
	// held the method list for the life of that stream, so no reverse mapping
	// is stored to reconstruct it.
	Remove(ctx context.Context, address string, methods []string) error

	// RemoveFromMethod takes address out of one method's set and answers with
	// the anchor that address was registered under, so the caller can tell that
	// instance what it did.
	//
	// This is the client-reported path: a client knows the address it could not
	// reach and the method it asked for, and nothing else that address serves.
	// Those other methods are removed the same way, by a client failing on them.
	RemoveFromMethod(ctx context.Context, method, address string) (anchor string, err error)

	// AddressesFor walks the addresses serving method. The sequence ends after
	// the last address, or early if the caller stops reading, so a method with
	// thousands of addresses is never materialized to answer one lookup.
	//
	// A method with no addresses yields nothing.
	AddressesFor(ctx context.Context, method string) iter.Seq2[string, error]

	// Notify tells the instance identified by anchor which row was removed.
	Notify(ctx context.Context, anchor string, removal Removal) error

	// Watch delivers the rows this instance anchored that something else
	// removed. It stops when ctx is done.
	Watch(ctx context.Context, anchor string) (<-chan Removal, error)

	// WatchMethod delivers addresses added to method after the call. It stops
	// when ctx is done.
	//
	// A Discover that has run out of candidates waits on this rather than
	// answering, so a consumer whose backend is entirely down is woken by the
	// next registration instead of asking again.
	WatchMethod(ctx context.Context, method string) (<-chan string, error)

	// Ping checks storage connectivity
	Ping(ctx context.Context) error

	// Name of the storage implementation
	Name() string

	// Address of the storage if applicable
	Address() string
}
