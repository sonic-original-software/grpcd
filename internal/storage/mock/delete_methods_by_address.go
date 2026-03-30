package mock

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// DeleteMethodsByAddress removes all methods registered by a specific address
func (s *Store) DeleteMethodsByAddress(_ context.Context, address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for injected error
	if s.deleteMethodsByAddressErr != nil {
		return s.deleteMethodsByAddressErr
	}

	addressKey := storage.AddressKey(address)

	// Get address set (like Redis SMEMBERS)
	methodSet := s.addressKeys[addressKey]
	if methodSet == nil {
		return nil // Idempotent
	}

	// Delete all method keys (like Redis pipeline DEL)
	for method := range methodSet {
		methodKey := storage.MethodKey(method)
		delete(s.methodKeys, methodKey)
	}

	// Delete the address set itself (like Redis DEL)
	delete(s.addressKeys, addressKey)

	return nil
}
