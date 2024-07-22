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

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner *TestMiner

	nextNulls         int64
	postWatchMiners   []address.Address
	postWatchMinersLk sync.Mutex
	pause             chan struct{}
	unpause           chan struct{}
	wg                sync.WaitGroup
	cancel            context.CancelFunc
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

type minerDeadline struct {
	addr     address.Address
	deadline dline.Info
}

type minerDeadlines []minerDeadline

func (mds minerDeadlines) CloseList() []abi.ChainEpoch {
	var ret []abi.ChainEpoch
	for _, md := range mds {
		ret = append(ret, md.deadline.Last())
	}
	return ret
}

func (mds minerDeadlines) MinerStringList() []string {
	var ret []string
	for _, md := range mds {
		ret = append(ret, md.addr.String())
	}
	return ret
}

// FilterByLast returns a new minerDeadlines with only the deadlines that have a Last() epoch
// greater than or equal to last.
func (mds minerDeadlines) FilterByLast(last abi.ChainEpoch) minerDeadlines {
	var ret minerDeadlines
	for _, md := range mds {
		if last >= md.deadline.Last() {
			ret = append(ret, md)
		}
	}
	return ret
}

type partitionTracker struct {
	minerAddr  address.Address
	partitions []api.Partition
	posted     bitfield.BitField
}

// newPartitionTracker creates a new partitionTracker that tracks the deadline index dlIdx for the
// given minerAddr. It uses the BlockMiner bm to interact with the chain.
func newPartitionTracker(ctx context.Context, t *testing.T, client v1api.FullNode, minerAddr address.Address, dlIdx uint64) *partitionTracker {
	dlines, err := client.StateMinerDeadlines(ctx, minerAddr, types.EmptyTSK)
	require.NoError(t, err)
	dl := dlines[dlIdx]

	parts, err := client.StateMinerPartitions(ctx, minerAddr, dlIdx, types.EmptyTSK)
	require.NoError(t, err)

	return &partitionTracker{
		minerAddr:  minerAddr,
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

func (p *partitionTracker) recordIfPost(t *testing.T, msg *types.Message) (ret bool) {
	defer func() {
		ret = p.done(t)
	}()
	if !(msg.To == p.minerAddr) {
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

func (bm *BlockMiner) forcePoSt(ctx context.Context, ts *types.TipSet, minerAddr address.Address, dlinfo dline.Info) {
	tracker := newPartitionTracker(ctx, bm.t, bm.miner.FullNode, minerAddr, dlinfo.Index)
	if !tracker.done(bm.t) { // need to wait for post
		bm.t.Logf("expect %d partitions proved but only see %d", len(tracker.partitions), tracker.count(bm.t))
		poolEvts, err := bm.miner.FullNode.MpoolSub(ctx) // subscribe before checking pending so we don't miss any events
		require.NoError(bm.t, err)

		// First check pending messages we'll mine this epoch
		msgs, err := bm.miner.FullNode.MpoolPending(ctx, types.EmptyTSK)
		require.NoError(bm.t, err)
		for _, msg := range msgs {
			if tracker.recordIfPost(bm.t, &msg.Message) {
				fmt.Printf("found post in mempool pending\n")
			}
		}

		// Account for included but not yet executed messages
		for _, bc := range ts.Cids() {
			msgs, err := bm.miner.FullNode.ChainGetBlockMessages(ctx, bc)
			require.NoError(bm.t, err)
			for _, msg := range msgs.BlsMessages {
				if tracker.recordIfPost(bm.t, msg) {
					fmt.Printf("found post in message of prev tipset\n")
				}

			}
			for _, msg := range msgs.SecpkMessages {
				if tracker.recordIfPost(bm.t, &msg.Message) {
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
						if tracker.recordIfPost(bm.t, &evt.Message.Message) {
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

// WatchMinerForPost adds a miner to the list of miners that the BlockMiner will watch for window
// post submissions when using MineBlocksMustPost. This is useful when we have more than just the
// BlockMiner submitting posts, particularly in the case of UnmanagedMiners which don't participate
// in block mining.
func (bm *BlockMiner) WatchMinerForPost(minerAddr address.Address) {
	bm.postWatchMinersLk.Lock()
	bm.postWatchMiners = append(bm.postWatchMiners, minerAddr)
	bm.postWatchMinersLk.Unlock()
}

// MineBlocksMustPost is like MineBlocks but refuses to mine until the window post scheduler has
// wdpost messages in the mempool and everything shuts down if a post fails.  It also enforces that
// every block mined succeeds
func (bm *BlockMiner) MineBlocksMustPost(ctx context.Context, blocktime time.Duration) {
	time.Sleep(time.Second)

	// watch for our own window posts
	bm.WatchMinerForPost(bm.miner.ActorAddr)

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

			// Get current deadline information for all miners, then filter by the ones that are about to
			// close so we can force a post for them.
			bm.postWatchMinersLk.Lock()
			var impendingDeadlines minerDeadlines
			for _, minerAddr := range bm.postWatchMiners {
				dlinfo, err := bm.miner.FullNode.StateMinerProvingDeadline(ctx, minerAddr, ts.Key())
				require.NoError(bm.t, err)
				require.NotNil(bm.t, dlinfo, "no deadline info for miner %s", minerAddr)
				if dlinfo.Open < dlinfo.CurrentEpoch {
					impendingDeadlines = append(impendingDeadlines, minerDeadline{addr: minerAddr, deadline: *dlinfo})
				} // else this is probably a new miner, not starting in this proving period
			}
			bm.postWatchMinersLk.Unlock()
			impendingDeadlines = impendingDeadlines.FilterByLast(ts.Height() + 5 + abi.ChainEpoch(nulls))

			if len(impendingDeadlines) > 0 {
				// Next block brings us too close for at least one deadline, we need to wait for miners to post
				bm.t.Logf("forcing post to get in if due before deadline closes at %v for %v", impendingDeadlines.CloseList(), impendingDeadlines.MinerStringList())
				for _, md := range impendingDeadlines {
					bm.forcePoSt(ctx, ts, md.addr, md.deadline)
				}
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
					// if we are mining a new null block and it brings us past deadline boundary we need to wait for miners to post
					impendingDeadlines = impendingDeadlines.FilterByLast(ts.Height() + 5 + abi.ChainEpoch(nulls+i))
					if len(impendingDeadlines) > 0 {
						bm.t.Logf("forcing post to get in if due before deadline closes at %v for %v", impendingDeadlines.CloseList(), impendingDeadlines.MinerStringList())
						for _, md := range impendingDeadlines {
							bm.forcePoSt(ctx, ts, md.addr, md.deadline)
						}
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

				require.NotEqual(bm.t, i, nloops-1, "block at height %d never managed to sync to node, which is at height %d", target, ts.Height())
				time.Sleep(time.Millisecond * 10)
			}

			switch {
			case err == nil: // wrap around
			case ctx.Err() != nil: // context fired.
				return
			default: // log error
				bm.t.Logf("MINEBLOCKS-post loop error: %+v", err)
				return
			}
		}
	}()

}

func (bm *BlockMiner) MineBlocks(ctx context.Context, blocktime time.Duration) {
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

			now := time.Duration(time.Now().UnixNano())
			delay := blocktime - (now % blocktime)

			select {
			case <-time.After(delay):
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
				bm.t.Logf("MINEBLOCKS loop error: %+v", err)
				return
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

				require.NotEqual(bm.t, i, nloops-1, "block at height %d never managed to sync to node, which is at height %d", epoch, ts.Height())
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
	bm.postWatchMinersLk.Lock()
	bm.postWatchMiners = nil
	bm.postWatchMinersLk.Unlock()
}
