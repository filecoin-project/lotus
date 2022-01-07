package kit

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	aminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
	"github.com/stretchr/testify/require"
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner *TestMiner

	nextNulls int64
	wg        sync.WaitGroup
	cancel    context.CancelFunc
}

func NewBlockMiner(t *testing.T, miner *TestMiner) *BlockMiner {
	return &BlockMiner{
		t:      t,
		miner:  miner,
		cancel: func() {},
	}
}

type partitionTracker struct {
	partitions []api.Partition
	posted     bitfield.BitField
}

func newPartitionTracker(ctx context.Context, dlIdx uint64, bm *BlockMiner) *partitionTracker {
	dlines, err := bm.miner.FullNode.StateMinerDeadlines(ctx, bm.miner.ActorAddr, types.EmptyTSK)
	require.NoError(bm.t, err)
	dl := dlines[dlIdx]

	parts, err := bm.miner.FullNode.StateMinerPartitions(ctx, bm.miner.ActorAddr, dlIdx, types.EmptyTSK)
	require.NoError(bm.t, err)
	return &partitionTracker{
		partitions: parts,
		posted:     dl.PostSubmissions,
	}
}

func (p *partitionTracker) count(t *testing.T) uint64 {
	pCnt, err := p.posted.Count()
	require.NoError(t, err)
	return pCnt
}

func (p *partitionTracker) done(t *testing.T) bool {
	return uint64(len(p.partitions)) == p.count(t)
}

func (p *partitionTracker) recordIfPost(t *testing.T, bm *BlockMiner, smsg *types.SignedMessage) (ret bool) {
	defer func() {
		ret = p.done(t)
	}()
	msg := smsg.Message
	if !(msg.To == bm.miner.ActorAddr) {
		return
	}
	if msg.Method != aminer.Methods.SubmitWindowedPoSt {
		return
	}
	params := aminer.SubmitWindowedPoStParams{}
	require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(msg.Params)))
	for _, part := range params.Partitions {
		p.posted.Set(part.Index)
	}
	return
}

// Like MineBlocks but refuses to mine until the window post scheduler has wdpost messages in the mempool
// and everything shuts down if a post fails.  It also enforces that every block mined succeeds
func (bm *BlockMiner) MineBlocksMustPost(ctx context.Context, blocktime time.Duration) {

	time.Sleep(3 * time.Second)

	// wrap context in a cancellable context.
	ctx, bm.cancel = context.WithCancel(ctx)
	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()

		activeDeadlines := make(map[int]struct{})
		_ = activeDeadlines
		ts, err := bm.miner.FullNode.ChainHead(ctx)
		require.NoError(bm.t, err)
		wait := make(chan bool)
		chg, err := bm.miner.FullNode.ChainNotify(ctx)
		require.NoError(bm.t, err)
		// read current out
		curr := <-chg
		require.Equal(bm.t, ts.Height(), curr[0].Val.Height())
		for {
			select {
			case <-time.After(blocktime):
			case <-ctx.Done():
				return
			}
			nulls := atomic.SwapInt64(&bm.nextNulls, 0)
			require.Equal(bm.t, int64(0), nulls, "Injecting > 0 null blocks while `MustPost` mining is currently unsupported")

			// Wake up and figure out if we are at the end of an active deadline
			ts, err := bm.miner.FullNode.ChainHead(ctx)
			require.NoError(bm.t, err)
			tsk := ts.Key()

			dlinfo, err := bm.miner.FullNode.StateMinerProvingDeadline(ctx, bm.miner.ActorAddr, tsk)
			require.NoError(bm.t, err)
			if ts.Height()+1 == dlinfo.Last() { // Last epoch in dline, we need to check that miner has posted

				tracker := newPartitionTracker(ctx, dlinfo.Index, bm)
				if !tracker.done(bm.t) { // need to wait for post
					bm.t.Logf("expect %d partitions proved but only see %d", len(tracker.partitions), tracker.count(bm.t))
					poolEvts, err := bm.miner.FullNode.MpoolSub(ctx)
					require.NoError(bm.t, err)

					// First check pending messages we'll mine this epoch
					msgs, err := bm.miner.FullNode.MpoolPending(ctx, types.EmptyTSK)
					require.NoError(bm.t, err)
					for _, msg := range msgs {
						tracker.recordIfPost(bm.t, bm, msg)
					}

					// post not yet in mpool, wait for it
					if !tracker.done(bm.t) {
						bm.t.Logf("post missing from mpool, block mining suspended until it arrives")
					POOL:
						for {
							bm.t.Logf("mpool event wait loop at block height %d, ts: %s", ts.Height(), ts.Key())
							select {
							case <-ctx.Done():
								return
							case evt := <-poolEvts:
								bm.t.Logf("pool event: %d", evt.Type)
								if evt.Type == api.MpoolAdd {
									bm.t.Logf("incoming message %v", evt.Message)
									if tracker.recordIfPost(bm.t, bm, evt.Message) {
										break POOL
									}
								}
							}
						}
						bm.t.Logf("done waiting on mpool")
					}
				}
			}

			var target abi.ChainEpoch
			reportSuccessFn := func(success bool, epoch abi.ChainEpoch, err error) {
				require.NoError(bm.t, err)
				target = epoch
				wait <- success
			}

			var success bool
			for i := int64(0); !success; i++ {
				err = bm.miner.MineOne(ctx, miner.MineReq{
					InjectNulls: abi.ChainEpoch(nulls + i),
					Done:        reportSuccessFn,
				})
				success = <-wait
			}

			// Wait until it shows up on the given full nodes ChainHead
			// TODO this replicates a flaky condition from MineUntil,
			// it would be better to use api to wait for sync,
			// but currently this is a bit difficult
			// and flaky failure is easy to debug and retry
			nloops := 200
			for i := 0; i < nloops; i++ {
				ts, err := bm.miner.FullNode.ChainHead(ctx)
				require.NoError(bm.t, err)

				if ts.Height() == target {
					break
				}

				require.NotEqual(bm.t, i, nloops-1, "block never managed to sync to node")
				time.Sleep(time.Millisecond * 10)
			}

			switch {
			case err == nil: // wrap around
			case ctx.Err() != nil: // context fired.
				return
			default: // log error
				bm.t.Error(err)
			}
		}
	}()

}

func (bm *BlockMiner) MineBlocks(ctx context.Context, blocktime time.Duration) {
	time.Sleep(time.Second)

	// wrap context in a cancellable context.
	ctx, bm.cancel = context.WithCancel(ctx)

	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()

		for {
			select {
			case <-time.After(blocktime):
			case <-ctx.Done():
				return
			}

			nulls := atomic.SwapInt64(&bm.nextNulls, 0)
			err := bm.miner.MineOne(ctx, miner.MineReq{
				InjectNulls: abi.ChainEpoch(nulls),
				Done:        func(bool, abi.ChainEpoch, error) {},
			})
			switch {
			case err == nil: // wrap around
			case ctx.Err() != nil: // context fired.
				return
			default: // log error
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

func (bm *BlockMiner) MineUntilBlock(ctx context.Context, fn *TestFullNode, cb func(abi.ChainEpoch)) {
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
			nloops := 200
			for i := 0; i < nloops; i++ {
				ts, err := fn.ChainHead(ctx)
				require.NoError(bm.t, err)

				if ts.Height() == epoch {
					break
				}

				require.NotEqual(bm.t, i, nloops-1, "block never managed to sync to node")
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
	bm.cancel()
	bm.wg.Wait()
}
