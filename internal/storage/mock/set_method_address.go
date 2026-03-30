package mock

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// SetMethodAddress stores a method → address mapping with TTL
func (s *Store) SetMethodAddress(_ context.Context, method, address string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for injected error
	if s.setMethodAddressErr != nil {
		return s.setMethodAddressErr
	}

	methodKey := storage.MethodKey(method)
	addressKey := storage.AddressKey(address)

	// Store method → address with expiry (like Redis SETEX)
	s.methodKeys[methodKey] = &entry{
		value:     address,
		expiresAt: time.Now().Add(ttl),
	}

	// Add method to address's reverse index set (like Redis SADD)
	if s.addressKeys[addressKey] == nil {
		s.addressKeys[addressKey] = make(set)
	}
	s.addressKeys[addressKey][method] = true

	return nil
}
