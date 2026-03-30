package mock

// SetSetMethodAddressError sets an error to be returned by SetMethodAddress
func (s *Store) SetSetMethodAddressError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setMethodAddressErr = err
}

// SetGetMethodAddressError sets an error to be returned by GetMethodAddress
func (s *Store) SetGetMethodAddressError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getMethodAddressErr = err
}

// SetDeleteMethodAddressError sets an error to be returned by DeleteMethodAddress
func (s *Store) SetDeleteMethodAddressError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteMethodAddressErr = err
}

// SetGetMethodsByAddressError sets an error to be returned by GetMethodsByAddress
func (s *Store) SetGetMethodsByAddressError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getMethodsByAddressErr = err
}

// SetDeleteMethodsByAddressError sets an error to be returned by DeleteMethodsByAddress
func (s *Store) SetDeleteMethodsByAddressError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteMethodsByAddressErr = err
}

// SetPingError sets an error to be returned by Ping
func (s *Store) SetPingError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingErr = err
}
