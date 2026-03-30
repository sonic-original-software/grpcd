package mock

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// GetMethodsByAddress retrieves all methods registered by a specific address
func (s *Store) GetMethodsByAddress(_ context.Context, address string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check for injected error
	if s.getMethodsByAddressErr != nil {
		return nil, s.getMethodsByAddressErr
	}

	addressKey := storage.AddressKey(address)

	// Get address set (like Redis SMEMBERS)
	methodSet := s.addressKeys[addressKey]
	if methodSet == nil {
		return []string{}, nil
	}

	methods := make([]string, 0, len(methodSet))

	// Only include non-expired methods
	for method := range methodSet {
		methodKey := storage.MethodKey(method)
		if e := s.methodKeys[methodKey]; e != nil {
			if !time.Now().After(e.expiresAt) {
				methods = append(methods, method)
			}
		}
	}

	return methods, nil
}
