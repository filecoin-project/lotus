package lazy

import (
	"context"
	"errors"
	"sync"
)

type Lazy[T any] struct {
	Get func() (T, error)

	once sync.Once

	val T
	err error
}

func MakeLazy[T any](get func() (T, error)) *Lazy[T] {
	return &Lazy[T]{
		Get: get,
	}
}

func (l *Lazy[T]) Val() (T, error) {
	l.once.Do(func() {
		l.val, l.err = l.Get()
	})
	return l.val, l.err
}

type LazyCtx[T any] struct {
	Get func(context.Context) (T, error)

	lk   sync.Mutex
	done bool

	val T
	err error
}

func MakeLazyCtx[T any](get func(ctx context.Context) (T, error)) *LazyCtx[T] {
	return &LazyCtx[T]{
		Get: get,
	}
}

// Val calls Get once and caches the result. A failed call whose context was
// cancelled is not cached, so the next caller with a live context calls Get again.
func (l *LazyCtx[T]) Val(ctx context.Context) (T, error) {
	l.lk.Lock()
	defer l.lk.Unlock()

	if !l.done {
		val, err := l.Get(ctx)
		if err != nil && errors.Is(ctx.Err(), context.Canceled) {
			return val, err
		}
		l.val, l.err, l.done = val, err, true
	}
	return l.val, l.err
}
