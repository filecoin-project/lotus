package kit

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/miner"
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner TestStorageNode

	nextNulls int64
	stopCh    chan chan struct{}
}

func NewBlockMiner(t *testing.T, miner TestStorageNode) *BlockMiner {
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
