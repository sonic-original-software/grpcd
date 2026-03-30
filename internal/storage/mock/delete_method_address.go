package mock

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// DeleteMethodAddress removes a specific method → address mapping
func (s *Store) DeleteMethodAddress(_ context.Context, method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for injected error
	if s.deleteMethodAddressErr != nil {
		return s.deleteMethodAddressErr
	}

	methodKey := storage.MethodKey(method)

	// Get entry to find address (like Redis GET before DELETE)
	e, exists := s.methodKeys[methodKey]
	if !exists {
		return nil // Idempotent
	}

	address := e.value
	addressKey := storage.AddressKey(address)

	// Remove from reverse index set (like Redis SREM)
	if addressSet := s.addressKeys[addressKey]; addressSet != nil {
		delete(addressSet, method)
		// Clean up empty set
		if len(addressSet) == 0 {
			delete(s.addressKeys, addressKey)
		}
	}

	// Delete method key (like Redis DEL)
	delete(s.methodKeys, methodKey)

	return nil
}
