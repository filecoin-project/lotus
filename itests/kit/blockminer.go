package kit

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner *TestMiner

	nextNulls int64
	pause     chan struct{}
	unpause   chan struct{}
	wg        sync.WaitGroup
	cancel    context.CancelFunc
}

func NewBlockMiner(t *testing.T, miner *TestMiner) *BlockMiner {
	return &BlockMiner{
		t:       t,
		miner:   miner,
		cancel:  func() {},
		unpause: make(chan struct{}),
		pause:   make(chan struct{}),
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

func (p *partitionTracker) recordIfPost(t *testing.T, bm *BlockMiner, msg *types.Message) (ret bool) {
	defer func() {
		ret = p.done(t)
	}()
	if !(msg.To == bm.miner.ActorAddr) {
		return
	}
	if msg.Method != builtin.MethodsMiner.SubmitWindowedPoSt {
		return
	}
	params := minertypes.SubmitWindowedPoStParams{}
	require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(msg.Params)))
	for _, part := range params.Partitions {
		p.posted.Set(part.Index)
	}
	return
}

func (bm *BlockMiner) forcePoSt(ctx context.Context, ts *types.TipSet, dlinfo *dline.Info) {

	tracker := newPartitionTracker(ctx, dlinfo.Index, bm)
	if !tracker.done(bm.t) { // need to wait for post
		bm.t.Logf("expect %d partitions proved but only see %d", len(tracker.partitions), tracker.count(bm.t))
		poolEvts, err := bm.miner.FullNode.MpoolSub(ctx) //subscribe before checking pending so we don't miss any events
		require.NoError(bm.t, err)

		// First check pending messages we'll mine this epoch
		msgs, err := bm.miner.FullNode.MpoolPending(ctx, types.EmptyTSK)
		require.NoError(bm.t, err)
		for _, msg := range msgs {
			if tracker.recordIfPost(bm.t, bm, &msg.Message) {
				fmt.Printf("found post in mempool pending\n")
			}
		}

		// Account for included but not yet executed messages
		for _, bc := range ts.Cids() {
			msgs, err := bm.miner.FullNode.ChainGetBlockMessages(ctx, bc)
			require.NoError(bm.t, err)
			for _, msg := range msgs.BlsMessages {
				if tracker.recordIfPost(bm.t, bm, msg) {
					fmt.Printf("found post in message of prev tipset\n")
				}

			}
			for _, msg := range msgs.SecpkMessages {
				if tracker.recordIfPost(bm.t, bm, &msg.Message) {
					fmt.Printf("found post in message of prev tipset\n")
				}
			}
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
						if tracker.recordIfPost(bm.t, bm, &evt.Message.Message) {
							fmt.Printf("found post in mempool evt\n")
							break POOL
						}
					}
				}
			}
			bm.t.Logf("done waiting on mpool")
		}
	}
}

// Like MineBlocks but refuses to mine until the window post scheduler has wdpost messages in the mempool
// and everything shuts down if a post fails.  It also enforces that every block mined succeeds
func (bm *BlockMiner) MineBlocksMustPost(ctx context.Context, blocktime time.Duration) {
	time.Sleep(time.Second)

	// wrap context in a cancellable context.
	ctx, bm.cancel = context.WithCancel(ctx)
	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()

		ts, err := bm.miner.FullNode.ChainHead(ctx)
		require.NoError(bm.t, err)
		wait := make(chan bool)
		chg, err := bm.miner.FullNode.ChainNotify(ctx)
		require.NoError(bm.t, err)
		// read current out
		curr := <-chg
		require.Equal(bm.t, ts.Height(), curr[0].Val.Height(), "failed sanity check: are multiple miners mining with must post?")
		for {
			select {
			case <-time.After(blocktime):
			case <-ctx.Done():
				return
			}
			nulls := atomic.SwapInt64(&bm.nextNulls, 0)

			// Wake up and figure out if we are at the end of an active deadline
			ts, err := bm.miner.FullNode.ChainHead(ctx)
			require.NoError(bm.t, err)

			dlinfo, err := bm.miner.FullNode.StateMinerProvingDeadline(ctx, bm.miner.ActorAddr, ts.Key())
			require.NoError(bm.t, err)
			if ts.Height()+1+abi.ChainEpoch(nulls) >= dlinfo.Last() { // Next block brings us past the last epoch in dline, we need to wait for miner to post
				bm.forcePoSt(ctx, ts, dlinfo)
			}

			var target abi.ChainEpoch
			reportSuccessFn := func(success bool, epoch abi.ChainEpoch, err error) {
				// if api shuts down before mining, we may get an error which we should probably just ignore
				// (fixing it will require rewriting most of the mining loop)
				if err != nil && ctx.Err() == nil && !strings.Contains(err.Error(), "websocket connection closed") && !api.ErrorIsIn(err, []error{new(jsonrpc.RPCConnectionError)}) {
					require.NoError(bm.t, err)
				}

				target = epoch
				select {
				case wait <- success:
				case <-ctx.Done():
				}
			}

			var success bool
			for i := int64(0); !success; i++ {
				err = bm.miner.MineOne(ctx, miner.MineReq{
					InjectNulls: abi.ChainEpoch(nulls + i),
					Done:        reportSuccessFn,
				})
				select {
				case success = <-wait:
				case <-ctx.Done():
					return
				}
				if !success {
					// if we are mining a new null block and it brings us past deadline boundary we need to wait for miner to post
					if ts.Height()+1+abi.ChainEpoch(nulls+i) >= dlinfo.Last() {
						bm.forcePoSt(ctx, ts, dlinfo)
					}
				}
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
			case <-bm.pause:
				select {
				case <-bm.unpause:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			default:
			}

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

// Pause compels the miner to wait for a signal to restart
func (bm *BlockMiner) Pause() {
	bm.pause <- struct{}{}
}

// Restart continues mining after a pause. This will hang if called before pause
func (bm *BlockMiner) Restart() {
	bm.unpause <- struct{}{}
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
	if bm.unpause != nil {
		close(bm.unpause)
		bm.unpause = nil
	}
	if bm.pause != nil {
		close(bm.pause)
		bm.pause = nil
	}
}
