package lazy

import (
	"context"
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

	once sync.Once

	val T
	err error
}

func MakeLazyCtx[T any](get func(ctx context.Context) (T, error)) *LazyCtx[T] {
	return &LazyCtx[T]{
		Get: get,
	}
}

func (l *LazyCtx[T]) Val(ctx context.Context) (T, error) {
	l.once.Do(func() {
		l.val, l.err = l.Get(ctx)
	})
	return l.val, l.err
}
