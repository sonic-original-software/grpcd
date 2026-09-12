package mock

// SetAddError sets an error to be returned by Add
func (s *Store) SetAddError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addErr = err
}

// SetRemoveError sets an error to be returned by Remove
func (s *Store) SetRemoveError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeErr = err
}

// SetRemoveFromMethodError sets an error to be returned by RemoveFromMethod
func (s *Store) SetRemoveFromMethodError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeFromMethodErr = err
}

// SetAddressesForError sets an error to be yielded by AddressesFor
func (s *Store) SetAddressesForError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addressesForErr = err
}

// SetCountError sets an error to be returned by Count
func (s *Store) SetCountError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countErr = err
}

// SetNotifyError sets an error to be returned by Notify
func (s *Store) SetNotifyError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifyErr = err
}

// SetWatchError sets an error to be returned by Watch
func (s *Store) SetWatchError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchErr = err
}

// Lose stages the store being unreachable. Pair it with an injected error so
// the handler under test both fails and finds the store lost.
func (s *Store) Lose() {
	s.conditions.Lose()
}

// Recover stages the store coming back.
func (s *Store) Recover() {
	s.conditions.Recover()
}

// SetPingError sets an error to be returned by Ping
func (s *Store) SetPingError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingErr = err
}
