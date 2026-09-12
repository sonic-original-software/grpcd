package storage

import "sync/atomic"

// Addition is one registration announced to this instance: what registered,
// and a channel closed when a newer one is announced.
//
// A handler waiting for a registration sleeps on Done. Closing it wakes every
// sleeper at once, with no record of who they were, and a sleeper that is gone
// costs nothing. What woke it is read from the next Latest.
type Addition struct {
	Method  string
	Address string
	Done    chan struct{}
}

// Additions holds the most recent Addition announced to this instance. One
// goroutine announces; any number of handlers read.
type Additions struct {
	latest atomic.Pointer[Addition]
}

// NewAdditions starts with an announcement of nothing, whose Done is open, so
// a handler has something to sleep on before anything has registered.
func NewAdditions() *Additions {
	a := &Additions{}
	a.latest.Store(&Addition{Done: make(chan struct{})})

	return a
}

// Latest answers with the most recent announcement. Load it before reading a
// method's set: an addition that lands after the read closes this one's Done,
// so the wait that follows is not slept through.
func (a *Additions) Latest() *Addition {
	return a.latest.Load()
}

// Announce records that address registered for method and wakes everything
// sleeping on the previous announcement. The new Addition is complete before
// it is published and never written to afterwards, so a woken handler reads
// it without coordination.
func (a *Additions) Announce(method, address string) {
	next := &Addition{Method: method, Address: address, Done: make(chan struct{})}

	previous := a.latest.Swap(next)

	close(previous.Done)
}
