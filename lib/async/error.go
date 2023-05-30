package async

// based on https://github.com/Gurpartap/async
// Apache-2.0 License, see https://github.com/Gurpartap/async/blob/master/License.txt

import (
	"context"

	"golang.org/x/xerrors"
)

type ErrorFuture interface {
	Await() error
	AwaitContext(ctx context.Context) error
}

type errorFuture struct {
	await func(ctx context.Context) error
}

func (f errorFuture) Await() error {
	return f.await(context.Background())
}

func (f errorFuture) AwaitContext(ctx context.Context) error {
	return f.await(ctx)
}

func Err(f func() error) ErrorFuture {
	var err error
	c := make(chan struct{}, 1)
	go func() {
		defer close(c)
		defer func() {
			if rerr := recover(); rerr != nil {
				err = xerrors.Errorf("async error: %s", rerr)
				return
			}
		}()
		err = f()
	}()
	return errorFuture{
		await: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c:
				return err
			}
		},
	}
}
