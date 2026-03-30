//revive:disable:package-comments
package mock

import (
	"sync"
	"time"
)

// entry stores a value with expiry time
type entry struct {
	value     string
	expiresAt time.Time
}

// set stores a set of strings
type set map[string]bool

// Store is an in-memory implementation of storage.Store for testing
// It behaves like Redis by using string keys internally
type Store struct {
	mu          sync.RWMutex
	methodKeys  map[string]*entry // method keys → address with TTL
	addressKeys map[string]set    // address keys → set of methods

	// Error injection for testing
	setMethodAddressErr       error
	getMethodAddressErr       error
	deleteMethodAddressErr    error
	getMethodsByAddressErr    error
	deleteMethodsByAddressErr error
	pingErr                   error
}

// NewStore creates a new in-memory store
func NewStore() *Store {
	return &Store{
		methodKeys:  make(map[string]*entry),
		addressKeys: make(map[string]set),
	}
}

// Name of the store
func (*Store) Name() string {
	return "mock"
}

// Address of the store
func (*Store) Address() string {
	return ""
}
