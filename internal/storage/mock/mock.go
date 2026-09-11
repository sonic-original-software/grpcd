//revive:disable:package-comments
package mock

import (
	"context"
	"iter"
	"slices"
	"sync"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// set stores a set of strings
type set map[string]bool

// Store is an in-memory implementation of storage.Store for testing.
//
// It holds the same three facts the Redis store holds — which addresses serve a
// method, which instance anchors an address, and the removals an anchor has
// been told about — so a test exercises the service layer without a backend.
type Store struct {
	mu sync.RWMutex

	methods  map[string]set    // method → addresses
	anchors  map[string]string // address → anchor
	watchers map[string]chan storage.Removal

	// methodWatchers holds every Discover waiting for a method to gain an
	// address. Several can wait on one method, so it is a set of channels.
	methodWatchers map[string]map[chan string]bool

	// watching is closed per method once something is waiting on it, so a test
	// can add an address after the wait has begun rather than before.
	watching map[string]chan struct{}

	// Error injection for testing
	addErr              error
	removeErr           error
	removeFromMethodErr error
	addressesForErr     error
	notifyErr           error
	watchErr            error
	watchMethodErr      error
	pingErr             error
}

// NewStore creates a new in-memory store
func NewStore() *Store {
	return &Store{
		methods:        make(map[string]set),
		anchors:        make(map[string]string),
		watchers:       make(map[string]chan storage.Removal),
		methodWatchers: make(map[string]map[chan string]bool),
		watching:       make(map[string]chan struct{}),
	}
}

// Add records that address serves each of methods, anchored to anchor, and
// announces the address to anything waiting on those methods.
func (s *Store) Add(_ context.Context, address, anchor string, methods []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.addErr != nil {
		return s.addErr
	}

	for _, method := range methods {
		if s.methods[method] == nil {
			s.methods[method] = make(set)
		}

		s.methods[method][address] = true

		// Fire and forget, as Pub/Sub is: a watcher that is not ready to
		// receive misses it.
		for watcher := range s.methodWatchers[method] {
			select {
			case watcher <- address:
			default:
			}
		}
	}

	s.anchors[address] = anchor

	return nil
}

// Remove takes address out of each of methods and forgets its anchor.
func (s *Store) Remove(_ context.Context, address string, methods []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.removeErr != nil {
		return s.removeErr
	}

	for _, method := range methods {
		delete(s.methods[method], address)

		if len(s.methods[method]) == 0 {
			delete(s.methods, method)
		}
	}

	delete(s.anchors, address)

	return nil
}

// RemoveFromMethod takes address out of one method's set, answering with the
// anchor it was registered under.
func (s *Store) RemoveFromMethod(_ context.Context, method, address string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.removeFromMethodErr != nil {
		return "", s.removeFromMethodErr
	}

	anchor := s.anchors[address]

	delete(s.methods[method], address)

	if len(s.methods[method]) == 0 {
		delete(s.methods, method)
	}

	return anchor, nil
}

// AddressesFor walks the addresses serving method, in a stable order so a test
// asserting on candidates does not depend on map iteration.
func (s *Store) AddressesFor(_ context.Context, method string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		s.mu.RLock()
		addresses := slices.Sorted(maps(s.methods[method]))
		err := s.addressesForErr
		s.mu.RUnlock()

		if err != nil {
			yield("", err)

			return
		}

		for _, address := range addresses {
			if !yield(address, nil) {
				return
			}
		}
	}
}

// Notify delivers removal to whoever is watching anchor.
func (s *Store) Notify(_ context.Context, anchor string, removal storage.Removal) error {
	s.mu.RLock()
	watcher, watching := s.watchers[anchor]
	err := s.notifyErr
	s.mu.RUnlock()

	if err != nil {
		return err
	}

	if !watching {
		return nil
	}

	watcher <- removal

	return nil
}

// Watch delivers the rows removed from under anchor.
func (s *Store) Watch(ctx context.Context, anchor string) (<-chan storage.Removal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.watchErr != nil {
		return nil, s.watchErr
	}

	watcher := make(chan storage.Removal)
	s.watchers[anchor] = watcher

	go func() {
		<-ctx.Done()

		s.mu.Lock()
		defer s.mu.Unlock()

		delete(s.watchers, anchor)
	}()

	return watcher, nil
}

// WatchMethod delivers addresses added to method after the call.
func (s *Store) WatchMethod(ctx context.Context, method string) (<-chan string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.watchMethodErr != nil {
		return nil, s.watchMethodErr
	}

	// Buffered by one so Add never blocks on a watcher that is between reads.
	watcher := make(chan string, 1)

	if s.methodWatchers[method] == nil {
		s.methodWatchers[method] = make(map[chan string]bool)
	}

	s.methodWatchers[method][watcher] = true

	if signal, waited := s.watching[method]; waited {
		close(signal)
		delete(s.watching, method)
	}

	go func() {
		<-ctx.Done()

		s.mu.Lock()
		defer s.mu.Unlock()

		// Removed from the set before closing, under the same lock Add
		// iterates under, so nothing can send on it afterwards.
		delete(s.methodWatchers[method], watcher)
		close(watcher)
	}()

	return watcher, nil
}

// Watching answers with a channel closed once something is waiting on method.
// A test that wants to add an address after a Discover has begun waiting, rather
// than before, blocks on this first.
func (s *Store) Watching(method string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	signal := make(chan struct{})

	if len(s.methodWatchers[method]) > 0 {
		close(signal)

		return signal
	}

	s.watching[method] = signal

	return signal
}

// Addresses answers with the addresses serving method, for assertions.
func (s *Store) Addresses(method string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return slices.Sorted(maps(s.methods[method]))
}

// Anchor answers with the anchor recorded for address, for assertions.
func (s *Store) Anchor(address string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.anchors[address]
}

// Name of the store
func (*Store) Name() string {
	return "mock"
}

// Address of the store
func (*Store) Address() string {
	return ""
}

// maps yields a set's members, so slices.Sorted can order them.
func maps(members set) iter.Seq[string] {
	return func(yield func(string) bool) {
		for member := range members {
			if !yield(member) {
				return
			}
		}
	}
}
