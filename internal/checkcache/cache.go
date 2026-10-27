// Package checkcache shares upstream results only for the lifetime of one check.
package checkcache

import (
	"context"
	"sync"
)

type key struct{}
type cache struct{ entries sync.Map }
type entry[T any] struct {
	done  chan struct{}
	value T
	err   error
}

func NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, &cache{})
}

func Active(ctx context.Context) bool { return ctx.Value(key{}) != nil }

// Do reuses successes and failures, including concurrent requests. A new context
// starts with an empty cache; credentials and compatibility belong in the key.
func Do[T any](ctx context.Context, name string, fetch func() (T, error)) (value T, err error) {
	defer func() { RecordError(ctx, err) }()
	c, ok := ctx.Value(key{}).(*cache)
	if !ok {
		return fetch()
	}
	e := &entry[T]{done: make(chan struct{})}
	actual, loaded := c.entries.LoadOrStore(name, e)
	if loaded {
		cached := actual.(*entry[T])
		select {
		case <-cached.done:
			return cached.value, cached.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	e.value, e.err = fetch()
	close(e.done)
	return e.value, e.err
}
