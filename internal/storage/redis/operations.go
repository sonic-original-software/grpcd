package redis

import (
	"context"
	"iter"
	"strings"

	"github.com/redis/go-redis/v9"

	"git.sonicoriginal.software/grpcd/internal/storage"
)

// Add records that address serves each of methods, anchored to anchor.
//
// SADD leaves a member that is already present alone, so an instance rewriting
// a row it still holds performs the same write as the one that created it.
func (r *Store) Add(ctx context.Context, address, anchor string, methods []string) error {
	pipe := r.client.TxPipeline()

	for _, method := range methods {
		pipe.SAdd(ctx, methodKey(method), address)
	}

	pipe.Set(ctx, anchorKey(address), anchor, 0)

	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	// Announced after the write lands, so a waiting Discover that is woken by
	// this finds the address in the set when it looks.
	pipe = r.client.Pipeline()

	for _, method := range methods {
		pipe.Publish(ctx, additions(method), address)
	}

	_, err := pipe.Exec(ctx)

	return err
}

// Remove takes address out of each of methods and forgets its anchor.
func (r *Store) Remove(ctx context.Context, address string, methods []string) error {
	pipe := r.client.TxPipeline()

	for _, method := range methods {
		pipe.SRem(ctx, methodKey(method), address)
	}

	pipe.Del(ctx, anchorKey(address))

	_, err := pipe.Exec(ctx)

	return err
}

// RemoveFromMethod takes address out of one method's set, answering with the
// anchor it was registered under.
//
// The anchor is read before the removal so the caller has somewhere to send the
// notification even though the removal is what makes it worth sending.
func (r *Store) RemoveFromMethod(
	ctx context.Context, method, address string,
) (string, error) {
	pipe := r.client.TxPipeline()

	anchor := pipe.Get(ctx, anchorKey(address))
	pipe.SRem(ctx, methodKey(method), address)

	// A missing anchor key makes Exec report redis.Nil, which reports on the
	// Get rather than on the removal. The removal is what matters here.
	if _, err := pipe.Exec(ctx); err != nil && !isNil(err) {
		return "", err
	}

	value, err := anchor.Result()
	if isNil(err) {
		return "", nil
	}

	return value, err
}

// AddressesFor walks the addresses serving method.
//
// SSCAN pages, so a method with thousands of addresses costs the caller only
// the pages it reads before it stops.
func (r *Store) AddressesFor(ctx context.Context, method string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		iterator := r.client.SScan(ctx, methodKey(method), 0, "", 0).Iterator()

		for iterator.Next(ctx) {
			if !yield(iterator.Val(), nil) {
				return
			}
		}

		if err := iterator.Err(); err != nil {
			yield("", err)
		}
	}
}

// Notify tells the instance identified by anchor which row was removed.
//
// Method names are rejected if they carry whitespace, so a space separates the
// two halves unambiguously.
func (r *Store) Notify(ctx context.Context, anchor string, removal storage.Removal) error {
	payload := removal.Method + " " + removal.Address

	return r.client.Publish(ctx, channel(anchor), payload).Err()
}

// Watch delivers the addresses this instance anchored that something else
// removed.
//
// Subscribe registers the channel before returning, so a removal published
// after this call reaches the caller.
func (r *Store) Watch(ctx context.Context, anchor string) (<-chan storage.Removal, error) {
	return subscribe(ctx, r.client, channel(anchor), func(payload string) (storage.Removal, bool) {
		method, address, found := strings.Cut(payload, " ")

		return storage.Removal{Method: method, Address: address}, found
	})
}

// WatchMethod delivers addresses added to method after the call.
func (r *Store) WatchMethod(ctx context.Context, method string) (<-chan string, error) {
	return subscribe(ctx, r.client, additions(method), func(payload string) (string, bool) {
		return payload, true
	})
}

// subscribe holds a subscription to name for as long as ctx lives, decoding
// each payload with decode and delivering what it accepts.
//
// Subscribe registers the channel before returning, so a message published
// after this call reaches the caller.
func subscribe[T any](
	ctx context.Context,
	client *redis.Client,
	name string,
	decode func(string) (T, bool),
) (<-chan T, error) {
	subscription := client.Subscribe(ctx, name)

	if _, err := subscription.Receive(ctx); err != nil {
		return nil, err
	}

	delivered := make(chan T)

	go func() {
		defer close(delivered)
		defer subscription.Close()

		for message := range subscription.Channel() {
			value, ok := decode(message.Payload)
			if !ok {
				continue
			}

			select {
			case delivered <- value:
			case <-ctx.Done():
				return
			}
		}
	}()

	return delivered, nil
}
