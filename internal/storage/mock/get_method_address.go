package mock

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// GetMethodAddress retrieves the address for a method
func (s *Store) GetMethodAddress(_ context.Context, method string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check for injected error
	if s.getMethodAddressErr != nil {
		return "", s.getMethodAddressErr
	}

	methodKey := storage.MethodKey(method)

	// Get entry (like Redis GET)
	e, exists := s.methodKeys[methodKey]
	if !exists {
		return "", storage.ErrMethodNotFound
	}

	// Check if entry has expired (simulate TTL)
	if time.Now().After(e.expiresAt) {
		return "", storage.ErrMethodNotFound
	}

	return e.value, nil
}
