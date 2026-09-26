package kit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

// How long MineBlocksMustPost suspends block production for each forced post, and for each mined
// block to reach the node's head.
const (
	postWaitTimeoutMockProofs = 20 * time.Second
	// Real proofs share the machine with the test's own sealing and can be starved for minutes.
	postWaitTimeoutRealProofs = 5 * time.Minute
)

// BlockMiner is a utility that makes a test miner Mine blocks on a timer.
type BlockMiner struct {
	t     *testing.T
	miner *TestMiner

	nextNulls         int64
	postWatchMiners   []address.Address
	postWatchMinersLk sync.Mutex
	postWait          time.Duration
	// postFailure receives the error for a forced post that failed to arrive; the ensemble sets it
	// to cancel its shared failure context.
	postFailure func(error)
	// expectedFailure, when set, receives a MineBlocksMustPost failure in place of failing the test.
	expectedFailure atomic.Pointer[func(error)]
	pause           chan struct{}
	unpause         chan struct{}
	wg              sync.WaitGroup
	cancel          context.CancelFunc
}

func NewBlockMiner(t *testing.T, miner *TestMiner) *BlockMiner {
	return &BlockMiner{
		t:        t,
		miner:    miner,
		cancel:   func() {},
		unpause:  make(chan struct{}),
		pause:    make(chan struct{}),
		postWait: postWaitTimeoutRealProofs,
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
	minerAddr address.Address
	dlIdx     uint64
	// mustProve holds the indexes of the partitions that need a WindowPoSt this deadline.
	mustProve []uint64
	posted    bitfield.BitField
}

// newPartitionTracker creates a new partitionTracker that tracks the deadline index dlIdx for the
// given minerAddr. withRecoveries belongs to the managed miner, whose scheduler proves recovering
// sectors as well; the unmanaged post loop leaves them out.
func newPartitionTracker(ctx context.Context, client v1api.FullNode, minerAddr address.Address, dlIdx uint64, withRecoveries bool) (*partitionTracker, error) {
	dlines, err := client.StateMinerDeadlines(ctx, minerAddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("reading deadlines of miner %s: %w", minerAddr, err)
	}
	if dlIdx >= uint64(len(dlines)) {
		return nil, fmt.Errorf("miner %s has %d deadlines, no deadline %d", minerAddr, len(dlines), dlIdx)
	}
	mustProve, err := partitionsToProve(ctx, client, minerAddr, dlIdx, withRecoveries)
	if err != nil {
		return nil, err
	}

	return &partitionTracker{
		minerAddr: minerAddr,
		dlIdx:     dlIdx,
		mustProve: mustProve,
		posted:    dlines[dlIdx].PostSubmissions,
	}, nil
}

// partitionsToProve lists the partitions of a deadline that get a WindowPoSt: those with a live
// non-faulty sector, plus those with a recovering sector for the block-producing miner.
func partitionsToProve(ctx context.Context, client v1api.FullNode, minerAddr address.Address, dlIdx uint64, withRecoveries bool) ([]uint64, error) {
	parts, err := client.StateMinerPartitions(ctx, minerAddr, dlIdx, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("reading partitions of miner %s deadline %d: %w", minerAddr, dlIdx, err)
	}

	var mustProve []uint64
	for idx, part := range parts { // deadline partition AMTs are dense, so idx is the partition index
		toProve, err := bitfield.SubtractBitField(part.LiveSectors, part.FaultySectors)
		if err != nil {
			return nil, xerrors.Errorf("partition %d live minus faulty sectors: %w", idx, err)
		}
		if withRecoveries {
			toProve, err = bitfield.MergeBitFields(toProve, part.RecoveringSectors)
			if err != nil {
				return nil, xerrors.Errorf("partition %d recovering sectors: %w", idx, err)
			}
		}
		empty, err := toProve.IsEmpty()
		if err != nil {
			return nil, xerrors.Errorf("partition %d sectors to prove: %w", idx, err)
		}
		if empty {
			continue
		}
		mustProve = append(mustProve, uint64(idx))
	}
	return mustProve, nil
}

func (p *partitionTracker) count() (uint64, error) {
	return p.posted.Count()
}

func (p *partitionTracker) done() (bool, error) {
	for _, idx := range p.mustProve {
		posted, err := p.posted.IsSet(idx)
		if err != nil || !posted {
			return false, err
		}
	}
	return true, nil
}

// recordIfPost records the partitions msg proves if it is a WindowPoSt from the tracked miner, and
// reports whether every partition that must be proved now has been.
func (p *partitionTracker) recordIfPost(msg *types.Message) (bool, error) {
	if msg.To == p.minerAddr && msg.Method == builtin.MethodsMiner.SubmitWindowedPoSt {
		params := minertypes.SubmitWindowedPoStParams{}
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, xerrors.Errorf("decoding WindowPoSt params: %w", err)
		}
		// a miner may have a post for an adjacent deadline in flight
		if params.Deadline == p.dlIdx {
			for _, part := range params.Partitions {
				p.posted.Set(part.Index)
			}
		}
	}
	return p.done()
}

// includedPosts returns a tracker for minerAddr's deadline that credits the posts already executed
// in the head's parent state, which is what StateMinerDeadlines reads, and those included in ts,
// whose execution is still pending. It does not look at the mempool.
func (bm *BlockMiner) includedPosts(ctx context.Context, ts *types.TipSet, minerAddr address.Address, dlinfo dline.Info) (*partitionTracker, error) {
	tracker, err := newPartitionTracker(ctx, bm.miner.FullNode, minerAddr, dlinfo.Index, minerAddr == bm.miner.ActorAddr)
	if err != nil {
		return nil, err
	}
	for _, bc := range ts.Cids() {
		msgs, err := bm.miner.FullNode.ChainGetBlockMessages(ctx, bc)
		if err != nil {
			return nil, xerrors.Errorf("reading messages of block %s: %w", bc, err)
		}
		for _, msg := range msgs.BlsMessages {
			if _, err := tracker.recordIfPost(msg); err != nil {
				return nil, err
			}
		}
		for _, msg := range msgs.SecpkMessages {
			if _, err := tracker.recordIfPost(&msg.Message); err != nil {
				return nil, err
			}
		}
	}
	return tracker, nil
}

// forcePoSt suspends block production until minerAddr's post for this deadline is pending in the
// mempool or included in ts. It returns an error if the post has not arrived within postWait or the
// wait itself fails.
func (bm *BlockMiner) forcePoSt(ctx context.Context, ts *types.TipSet, minerAddr address.Address, dlinfo dline.Info) error {
	tracker, err := bm.includedPosts(ctx, ts, minerAddr, dlinfo)
	if err != nil {
		return err
	}
	if done, err := tracker.done(); done || err != nil {
		return err
	}

	postCtx, cancel := context.WithTimeout(ctx, bm.postWait)
	defer cancel()
	missing := func(err error) error {
		// done() has already decoded the bitfield, so count() can't fail here
		proved, _ := tracker.count()
		return xerrors.Errorf("no window post from miner %s for deadline %d (open %d, close %d) at height %d, %d of %d partitions proved: %w",
			minerAddr, dlinfo.Index, dlinfo.Open, dlinfo.Close, ts.Height(), proved, len(tracker.mustProve), err)
	}
	record := func(msg *types.Message, where string) (bool, error) {
		done, err := tracker.recordIfPost(msg)
		if err != nil {
			return false, missing(err)
		}
		if done {
			bm.t.Logf("found post for %s deadline %d %s", minerAddr, dlinfo.Index, where)
		}
		return done, nil
	}
	proved, _ := tracker.count()
	bm.t.Logf("expect %d partitions proved but only see %d", len(tracker.mustProve), proved)

	poolEvts, err := bm.miner.FullNode.MpoolSub(postCtx) // subscribe before checking pending so we don't miss any events
	if err != nil {
		return missing(err)
	}

	// First check pending messages we'll mine this epoch
	msgs, err := bm.miner.FullNode.MpoolPending(postCtx, types.EmptyTSK)
	if err != nil {
		return missing(err)
	}
	for _, msg := range msgs {
		if done, err := record(&msg.Message, "in mempool"); done || err != nil {
			return err
		}
	}

	bm.t.Logf("post missing from mpool, block mining suspended until it arrives")
WAIT:
	for {
		bm.t.Logf("mpool event wait loop at block height %d, ts: %s", ts.Height(), ts.Key())
		select {
		case <-postCtx.Done():
			break WAIT
		case evt, ok := <-poolEvts:
			if !ok {
				if postCtx.Err() == nil {
					return missing(errors.New("mpool subscription closed"))
				}
				break WAIT
			}
			bm.t.Logf("pool event: %d", evt.Type)
			if evt.Type == api.MpoolAdd {
				bm.t.Logf("incoming message %v", evt.Message)
				if done, err := record(&evt.Message.Message, "in a mempool event"); done || err != nil {
					return err
				}
			}
		}
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return missing(xerrors.Errorf("waited %s: %w", bm.postWait, postCtx.Err()))
}

// ExpectFailure hands the next MineBlocksMustPost failure to fn instead of failing the test, for
// tests of the failure path itself. Block production still stops and the ensemble's failure
// context is still cancelled with the failure.
func (bm *BlockMiner) ExpectFailure(fn func(error)) {
	bm.expectedFailure.Store(&fn)
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

// MineBlocksMustPost is like MineBlocks but refuses to mine a block that would take a watched miner
// past a window PoSt deadline before that miner's PoSt message is in the mempool. Every block mined
// must succeed. It's a brake that couples chain time to the PoSt scheduler, test chains can produce
// blocks faster than a scheduler can prove, so without it deadlines pass unproven, sectors fault,
// and a miner that loses its power can no longer win elections. Use it for tests whose sectors must
// keep their power across deadlines (onboarding, upgrades, extensions, terminations, fee checks),
// with managed or unmanaged miners.
//
// Each round:
//
//   - Read the current deadline of every watched miner (the BlockMiner's own and any added with
//     WatchMinerForPost) at the current head.
//   - Work out the epoch the next block will land on: the head, plus one, plus any null rounds
//     requested for this round.
//   - For each open deadline that epoch would bring too near to its close, wait until that miner's
//     PoSt message is pending in the mempool or already included, so the next block has it.
//   - Mine. *On a lost election* (more likely the more power other miners hold) the miner itself
//     counts a null round on the same mining base, so the next attempt lands one epoch later. Before
//     each retry the deadline check runs again for the new landing epoch, across all open deadlines,
//     so a losing streak can't mine past a deadline close without first forcing the PoSt that's due.
//   - Null rounds requested by the caller are injected once, on the first attempt; retries rely on
//     the miner's own null count.
//
// A PoSt that has not arrived within postWait, a mining error, or a mined block that never reaches
// the node's head fails the test and stops block production. The failure goes to postFailure, which
// for an ensemble's block miner cancels the ensemble's failure context, so waiters on that context
// return the error rather than wait on a stopped chain.
func (bm *BlockMiner) MineBlocksMustPost(ctx context.Context, blocktime time.Duration) {
	time.Sleep(time.Second)

	// watch for our own window posts
	bm.WatchMinerForPost(bm.miner.ActorAddr)

	// wrap context in a cancellable context.
	ctx, cancel := context.WithCancel(ctx)
	bm.cancel = cancel
	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()
		if err := bm.mineMustPost(ctx, blocktime); err != nil && ctx.Err() == nil {
			if expected := bm.expectedFailure.Load(); expected != nil {
				(*expected)(err)
			} else {
				bm.t.Error(err)
			}
			if bm.postFailure != nil {
				bm.postFailure(err)
			}
			cancel()
		}
	}()
}

// mineMustPost is the MineBlocksMustPost loop. It returns nil once ctx is done, or the error that
// stopped block production.
func (bm *BlockMiner) mineMustPost(ctx context.Context, blocktime time.Duration) error {
	ts, err := bm.miner.FullNode.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("reading chain head: %w", err)
	}
	wait := make(chan bool)
	chg, err := bm.miner.FullNode.ChainNotify(ctx)
	if err != nil {
		return xerrors.Errorf("subscribing to chain notifications: %w", err)
	}
	// read current out
	select {
	case curr, ok := <-chg:
		if !ok || len(curr) == 0 {
			return errors.New("chain notification closed before the current head")
		}
		if curr[0].Val.Height() != ts.Height() {
			return fmt.Errorf("failed sanity check: are multiple miners mining with must post? head at %d, notified %d",
				ts.Height(), curr[0].Val.Height())
		}
	case <-ctx.Done():
		return nil
	}
	for {
		select {
		case <-time.After(blocktime):
		case <-ctx.Done():
			return nil
		}
		nulls := atomic.SwapInt64(&bm.nextNulls, 0)

		// Wake up and figure out if we are at the end of an active deadline
		ts, err := bm.miner.FullNode.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("reading chain head: %w", err)
		}

		openDeadlines, err := bm.openDeadlines(ctx, ts)
		if err != nil {
			return err
		}

		// forceDue waits on every open deadline that a block landing at `landing` would bring
		// within four epochs of its last epoch. A deadline that has already closed by `landing` is
		// checked, not forced.
		forceDue := func(landing abi.ChainEpoch, attempt int64) error {
			var toForce minerDeadlines
			for _, md := range openDeadlines.FilterByLast(landing + 4) {
				dl := md.deadline
				if dl.Close <= landing {
					// The block cannot include a post for this deadline in time. Only a post executed
					// in the parent state or included in ts proves it; the mempool is not consulted,
					// as a post merely pending there can no longer land before the close.
					tracker, err := bm.includedPosts(ctx, ts, md.addr, dl)
					if err != nil {
						return err
					}
					proved, err := tracker.done()
					if err != nil {
						return err
					}
					if !proved {
						when := "during a losing streak"
						switch {
						case attempt > 0:
						case nulls > 0:
							when = "under requested null rounds"
						default:
							when = "before the next block"
						}
						return fmt.Errorf("deadline %d of miner %s closed unproven %s (open %d, close %d, base height %d, landing %d)",
							dl.Index, md.addr, when, dl.Open, dl.Close, ts.Height(), landing)
					}
					continue
				}
				toForce = append(toForce, md)
			}
			if len(toForce) == 0 {
				return nil
			}
			bm.t.Logf("forcing post to get in if due before deadline closes at %v for %v", toForce.CloseList(), toForce.MinerStringList())
			for _, md := range toForce {
				if err := bm.forcePoSt(ctx, ts, md.addr, md.deadline); err != nil {
					return err
				}
			}
			return nil
		}

		var (
			target  abi.ChainEpoch
			mineErr error
		)
		reportSuccessFn := func(success bool, epoch abi.ChainEpoch, err error) {
			// an API shutting down before mining may report an error, which is not a mining failure
			if err != nil && ctx.Err() == nil && !strings.Contains(err.Error(), "websocket connection closed") && !api.ErrorIsIn(err, []error{new(jsonrpc.RPCConnectionError)}) {
				mineErr = err
			}

			target = epoch
			select {
			case wait <- success:
			case <-ctx.Done():
			}
		}

		// The miner counts a lost round as a null round on the same base, so only the first attempt
		// injects nulls: attempt i lands at ts.Height() + 1 + nulls + i.
		var success bool
		for i := int64(0); !success; i++ {
			if err := forceDue(ts.Height()+1+abi.ChainEpoch(nulls+i), i); err != nil {
				return err
			}
			inject := abi.ChainEpoch(0)
			if i == 0 {
				inject = abi.ChainEpoch(nulls)
			}
			if err := bm.miner.MineOne(ctx, miner.MineReq{
				InjectNulls: inject,
				Done:        reportSuccessFn,
			}); err != nil {
				return xerrors.Errorf("requesting a block: %w", err)
			}
			select {
			case success = <-wait:
			case <-ctx.Done():
				return nil
			}
			if mineErr != nil {
				return xerrors.Errorf("mining a block on height %d: %w", ts.Height(), mineErr)
			}
		}

		if err := bm.waitForHead(ctx, target); err != nil {
			return err
		}
	}
}

// openDeadlines reads, at ts, the current deadline of every watched miner that's got one open.
func (bm *BlockMiner) openDeadlines(ctx context.Context, ts *types.TipSet) (minerDeadlines, error) {
	bm.postWatchMinersLk.Lock()
	defer bm.postWatchMinersLk.Unlock()

	var open minerDeadlines
	for _, minerAddr := range bm.postWatchMiners {
		dlinfo, err := bm.miner.FullNode.StateMinerProvingDeadline(ctx, minerAddr, ts.Key())
		if err != nil {
			return nil, xerrors.Errorf("reading proving deadline of miner %s at height %d: %w", minerAddr, ts.Height(), err)
		}
		if dlinfo == nil {
			return nil, fmt.Errorf("no deadline info for miner %s at height %d", minerAddr, ts.Height())
		}
		// Open at the head epoch counts: the scheduler has that head and can post for it. A new
		// miner's first proving period starts in the future, so its first deadline is not open yet.
		if dlinfo.IsOpen() {
			open = append(open, minerDeadline{addr: minerAddr, deadline: *dlinfo})
		}
	}
	return open, nil
}

// waitForHead waits, for at most postWait, until the miner's node has a head at or above target.
func (bm *BlockMiner) waitForHead(ctx context.Context, target abi.ChainEpoch) error {
	waitCtx, cancel := context.WithTimeout(ctx, bm.postWait)
	defer cancel()
	height := abi.ChainEpoch(-1)
	for {
		head, err := bm.miner.FullNode.ChainHead(waitCtx)
		if err == nil {
			if head.Height() >= target {
				return nil
			}
			height = head.Height()
		} else if waitCtx.Err() == nil {
			return xerrors.Errorf("waiting for block at height %d to sync to node: %w", target, err)
		}
		select {
		case <-waitCtx.Done():
			if err := context.Cause(ctx); err != nil {
				return err
			}
			return fmt.Errorf("block at height %d did not sync to node within %s, node last seen at height %d", target, bm.postWait, height)
		case <-time.After(10 * time.Millisecond):
		}
	}
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
