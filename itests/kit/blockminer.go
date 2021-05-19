package kit

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/miner"
	"github.com/stretchr/testify/require"
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner TestMiner

	nextNulls int64
	stopCh    chan chan struct{}
}

func NewBlockMiner(t *testing.T, miner TestMiner) *BlockMiner {
	return &BlockMiner{
		t:      t,
		miner:  miner,
		stopCh: make(chan chan struct{}),
	}
}

func (bm *BlockMiner) MineBlocks(ctx context.Context, blocktime time.Duration) {
	time.Sleep(time.Second)

	go func() {
		for {
			select {
			case <-time.After(blocktime):
			case <-ctx.Done():
				return
			case ch := <-bm.stopCh:
				close(ch)
				close(bm.stopCh)
				return
			}

			nulls := atomic.SwapInt64(&bm.nextNulls, 0)
			if err := bm.miner.MineOne(ctx, miner.MineReq{
				InjectNulls: abi.ChainEpoch(nulls),
				Done:        func(bool, abi.ChainEpoch, error) {},
			}); err != nil {
				bm.t.Error(err)
			}
		}
	}()
}

// InjectNulls injects the specified amount of null rounds in the next
// mining rounds.
func (bm *BlockMiner) InjectNulls(rounds abi.ChainEpoch) {
	atomic.AddInt64(&bm.nextNulls, int64(rounds))
}

func (bm *BlockMiner) MineUntilBlock(ctx context.Context, fn TestFullNode, cb func(abi.ChainEpoch)) {
	for i := 0; i < 1000; i++ {
		var (
			success bool
			err     error
			epoch   abi.ChainEpoch
			wait    = make(chan struct{})
		)

		doneFn := func(win bool, ep abi.ChainEpoch, e error) {
			success = win
			err = e
			epoch = ep
			wait <- struct{}{}
		}

		mineErr := bm.miner.MineOne(ctx, miner.MineReq{Done: doneFn})
		require.NoError(bm.t, mineErr)
		<-wait

		require.NoError(bm.t, err)

		if success {
			// Wait until it shows up on the given full nodes ChainHead
			nloops := 50
			for i := 0; i < nloops; i++ {
				ts, err := fn.ChainHead(ctx)
				require.NoError(bm.t, err)

				if ts.Height() == epoch {
					break
				}

				require.Equal(bm.t, i, nloops-1, "block never managed to sync to node")
				time.Sleep(time.Millisecond * 10)
			}

			if cb != nil {
				cb(epoch)
			}
			return
		}
		bm.t.Log("did not Mine block, trying again", i)
	}
	bm.t.Fatal("failed to Mine 1000 times in a row...")
}

// Stop stops the block miner.
func (bm *BlockMiner) Stop() {
	bm.t.Log("shutting down mining")
	if _, ok := <-bm.stopCh; ok {
		// already stopped
		return
	}
	ch := make(chan struct{})
	bm.stopCh <- ch
	<-ch
}
