package store

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

type countingLoader struct {
	byKey map[types.TipSetKey]*types.TipSet
	loads atomic.Int64
}

func newCountingLoader(n int) (*countingLoader, *types.TipSet) {
	l := &countingLoader{byKey: make(map[types.TipSetKey]*types.TipSet, n)}
	ts := mock.TipSet(mock.MkBlock(nil, 1, 1))
	l.byKey[ts.Key()] = ts
	for i := 1; i < n; i++ {
		ts = mock.TipSet(mock.MkBlock(ts, 1, 1))
		l.byKey[ts.Key()] = ts
	}
	return l, ts
}

func (l *countingLoader) load(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.loads.Add(1)
	ts, ok := l.byKey[tsk]
	if !ok {
		return nil, xerrors.Errorf("tipset %s not found", tsk)
	}
	return ts, nil
}

func TestChainIndexWarm(t *testing.T) {
	ctx := context.Background()
	loader, head := newCountingLoader(1000)

	cold := NewChainIndex(loader.load)
	_, err := cold.GetTipsetByHeight(ctx, head, 5)
	require.NoError(t, err)
	require.Greater(t, loader.loads.Load(), int64(900), "a cold lookup loads every tipset down to the target")

	ci := NewChainIndex(loader.load)
	require.Equal(t, abi.ChainEpoch(-1), ci.ReadyTo())
	require.NoError(t, ci.Warm(ctx, head))
	require.Equal(t, abi.ChainEpoch(0), ci.ReadyTo())

	loader.loads.Store(0)
	ts, err := ci.GetTipsetByHeight(ctx, head, 5)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(5), ts.Height())
	require.LessOrEqual(t, loader.loads.Load(), int64(2*ci.skipLength), "a warm lookup only walks within one skip")
}

func TestChainIndexWarmCancel(t *testing.T) {
	loader, head := newCountingLoader(1000)
	ctx, cancel := context.WithCancel(context.Background())

	ci := NewChainIndex(func(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
		if loader.loads.Load() == 100 {
			cancel()
		}
		return loader.load(ctx, tsk)
	})
	require.ErrorIs(t, ci.Warm(ctx, head), context.Canceled)
	require.Greater(t, ci.ReadyTo(), abi.ChainEpoch(800), "stops partway, keeping progress")
}
