package test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/miner"
)

type BlockMiner struct {
	ctx       context.Context
	t         *testing.T
	miner     TestStorageNode
	blocktime time.Duration
	mine      int64
	nulls     int64
	done      chan struct{}
}

func NewBlockMiner(ctx context.Context, t *testing.T, miner TestStorageNode, blocktime time.Duration) *BlockMiner {
	return &BlockMiner{
		ctx:       ctx,
		t:         t,
		miner:     miner,
		blocktime: blocktime,
		mine:      int64(1),
		done:      make(chan struct{}),
	}
}

func (bm *BlockMiner) MineBlocks() {
	time.Sleep(time.Second)
	go func() {
		defer close(bm.done)
		for atomic.LoadInt64(&bm.mine) == 1 {
			time.Sleep(bm.blocktime)
			nulls := atomic.SwapInt64(&bm.nulls, 0)
			if err := bm.miner.MineOne(bm.ctx, miner.MineReq{
				InjectNulls: abi.ChainEpoch(nulls),
				Done:        func(bool, abi.ChainEpoch, error) {},
			}); err != nil {
				bm.t.Error(err)
			}
		}
	}()
}

func (bm *BlockMiner) Stop() {
	atomic.AddInt64(&bm.mine, -1)
	fmt.Println("shutting down mining")
	<-bm.done
}
