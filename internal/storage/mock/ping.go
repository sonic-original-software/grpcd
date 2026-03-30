package mock

import "context"

// Ping checks storage connectivity
func (s *Store) Ping(_ context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pingErr != nil {
		return s.pingErr
	}

	// In-memory store is always available
	return nil
}
