package lazy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLazyCtxRetriesAfterCancelledCall(t *testing.T) {
	calls := 0
	l := MakeLazyCtx(func(ctx context.Context) (int, error) {
		calls++
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return 42, nil
	})

	cctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Val(cctx)
	require.ErrorIs(t, err, context.Canceled)

	v, err := l.Val(context.Background())
	require.NoError(t, err)
	require.Equal(t, 42, v)

	v, err = l.Val(cctx)
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.Equal(t, 2, calls)
}

func TestLazyCtxCachesOtherErrors(t *testing.T) {
	errUnreachable := errors.New("worker unreachable")

	tests := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		get  func(ctx context.Context) (int, error)
		want error
	}{
		{
			name: "error with a live context",
			ctx:  func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			get:  func(context.Context) (int, error) { return 0, errUnreachable },
			want: errUnreachable,
		},
		{
			name: "deadline exceeded",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now())
			},
			get:  func(ctx context.Context) (int, error) { return 0, ctx.Err() },
			want: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			l := MakeLazyCtx(func(ctx context.Context) (int, error) {
				calls++
				return tt.get(ctx)
			})

			ctx, cancel := tt.ctx()
			defer cancel()

			_, err := l.Val(ctx)
			require.ErrorIs(t, err, tt.want)

			_, err = l.Val(context.Background())
			require.ErrorIs(t, err, tt.want)
			require.Equal(t, 1, calls)
		})
	}
}
