package redis

import (
	"context"
	"iter"
	"strings"

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
	subscription := r.client.Subscribe(ctx, channel(anchor))

	if _, err := subscription.Receive(ctx); err != nil {
		return nil, err
	}

	removals := make(chan storage.Removal)

	go func() {
		defer close(removals)
		defer subscription.Close()

		for message := range subscription.Channel() {
			method, address, found := strings.Cut(message.Payload, " ")
			if !found {
				continue
			}

			select {
			case removals <- storage.Removal{Method: method, Address: address}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return removals, nil
}
