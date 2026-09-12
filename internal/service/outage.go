package service

import (
	"context"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// storeLost reports whether a failed operation is the store being lost, and if
// so blocks until the store's condition next changes, answering false if ctx
// ends first.
//
// The condition is read after the failure, because the failure is what
// records the loss: a condition read before it still says healthy. The
// condition read here is the lost one, so a recovery landing after it is
// read closes the channel waited on and is not slept through.
func storeLost(ctx context.Context, store storage.Store) (lost, waited bool) {
	condition := store.Condition()

	if !condition.Lost {
		return false, false
	}

	select {
	case <-condition.Changed:
		return true, true
	case <-ctx.Done():
		return true, false
	}
}
