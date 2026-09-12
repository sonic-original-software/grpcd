package redis

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// Listen subscribes this instance to every method's additions, once, and
// announces each as it arrives. Every handler waiting on a registration wakes
// from that one announcement, so the subscription count does not grow with
// the handlers waiting.
//
// The subscription is also how recovery is noticed. go-redis reconnects and
// resubscribes on its own after a network error, and pings a quiet link so a
// silent one is noticed, and each resubscription is delivered as a
// Subscription: the first is this call succeeding, every later one is the
// store coming back.
func (r *Store) Listen(ctx context.Context) error {
	subscription := r.client.PSubscribe(ctx, additionsPattern)

	messages := subscription.ChannelWithSubscriptions()

	// Registered before returning, so an addition published after this call
	// is announced.
	if err := confirmed(ctx, messages); err != nil {
		subscription.Close()

		return err
	}

	go func() {
		defer subscription.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case message, open := <-messages:
				if !open {
					return
				}

				r.announce(message)
			}
		}
	}()

	return nil
}

// confirmed waits for the first subscription confirmation.
func confirmed(ctx context.Context, messages <-chan interface{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case message, open := <-messages:
		if !open {
			return errors.New("subscription closed before it was confirmed")
		}

		if _, ok := message.(*redis.Subscription); !ok {
			return errors.New("subscription delivered a message before it was confirmed")
		}

		return nil
	}
}

// announce acts on one delivery: a resubscription means the store came back,
// a message means an address registered.
func (r *Store) announce(message interface{}) {
	switch m := message.(type) {
	case *redis.Subscription:
		r.conditions.Recover()
	case *redis.Message:
		r.additions.Announce(methodOf(m.Channel), m.Payload)
	}
}
