package chanutil

import (
	"context"
	"reflect"
)

// Collect will return up to one value from each channel. Will block until
// it receives a value from each channel, or until the context has been cancelled
func Collect[T any](ctx context.Context, chans []chan T) []*T {
	blocking := reflect.ValueOf(make(chan struct{}))

	cases := make([]reflect.SelectCase, len(chans)+1)
	for i, ch := range chans {
		cases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		}
	}

	cases[len(chans)] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}

	out := make([]*T, len(chans))

	for i := 0; i < len(chans); i++ {
		chonen, rv, ok := reflect.Select(cases)

		if chonen == len(chans) { // ctx.Done
			return out
		}

		if ok {
			val := rv.Interface().(T)
			out[chonen] = &val
		}

		cases[chonen].Chan = blocking
	}

	return out
}
