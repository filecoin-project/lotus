package lpwinning

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	logging "github.com/ipfs/go-log/v2"
	"os"
	"time"
)

var log = logging.Logger("lpwinning")

type WinPostTask struct {
	max abi.SectorNumber

	// lastWork holds the last MiningBase we built upon.
	lastWork *MiningBase

	api WinPostAPI
}

type WinPostAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainTipSetWeight(context.Context, types.TipSetKey) (types.BigInt, error)

	StateGetBeaconEntry(context.Context, abi.ChainEpoch) (*types.BeaconEntry, error)
	SyncSubmitBlock(context.Context, *types.BlockMsg) error
}

func NewWinPostTask(max abi.SectorNumber) *WinPostTask {
	// todo run warmup
}

func (t *WinPostTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	// TODO THIS WILL BASICALLY BE A mineOne() function

	//TODO implement me
	panic("implement me")
}

func (t *WinPostTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	//TODO implement me
	panic("implement me")
}

func (t *WinPostTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name:        "WinPost",
		Max:         10, // todo
		MaxFailures: 3,
		Follows:     nil,
		Cost: resources.Resources{
			Cpu: 1,

			// todo set to something for 32/64G sector sizes? Technically windowPoSt is happy on a CPU
			//  but it will use a GPU if available
			Gpu: 0,

			Ram: 1 << 30, // todo arbitrary number
		},
	}
}

func (t *WinPostTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	//TODO implement me
	panic("implement me")
}

// MiningBase is the tipset on top of which we plan to construct our next block.
// Refer to godocs on GetBestMiningCandidate.
type MiningBase struct {
	TipSet      *types.TipSet
	ComputeTime time.Time
	NullRounds  abi.ChainEpoch
}

func (t *WinPostTask) mine(ctx context.Context) {
	var lastBase MiningBase

	// Start the main mining loop.
	for {
		// todo handle stop signals?

		var base *MiningBase

		// Look for the best mining candidate.
		for {
			prebase, err := t.GetBestMiningCandidate(ctx)
			if err != nil {
				log.Errorf("failed to get best mining candidate: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Check if we have a new base or if the current base is still valid.
			if base != nil && base.TipSet.Height() == prebase.TipSet.Height() && base.NullRounds == prebase.NullRounds {
				// We have a valid base.
				base = prebase
				break
			}

			// TODO: need to change the orchestration here. the problem is that
			// we are waiting *after* we enter this loop and selecta mining
			// candidate, which is almost certain to change in multiminer
			// tests. Instead, we should block before entering the loop, so
			// that when the test 'MineOne' function is triggered, we pull our
			// best mining candidate at that time.

			// Wait until propagation delay period after block we plan to mine on
			{
				// if we're mining a block in the past via catch-up/rush mining,
				// such as when recovering from a network halt, this sleep will be
				// for a negative duration, and therefore **will return
				// immediately**.
				//
				// the result is that we WILL NOT wait, therefore fast-forwarding
				// and thus healing the chain by backfilling it with null rounds
				// rapidly.
				baseTs := prebase.TipSet.MinTimestamp() + build.PropagationDelaySecs
				baseT := time.Unix(int64(baseTs), 0)
				baseT = baseT.Add(randTimeOffset(time.Second))
				time.Sleep(time.Until(baseT))
			}

			// Ensure the beacon entry is available before finalizing the mining base.
			_, err = t.api.StateGetBeaconEntry(ctx, prebase.TipSet.Height()+prebase.NullRounds+1)
			if err != nil {
				log.Errorf("failed getting beacon entry: %s", err)
				time.Sleep(time.Second)
				continue
			}

			base = prebase
		}

		// Check for repeated mining candidates and handle sleep for the next round.
		if base.TipSet.Equals(lastBase.TipSet) && lastBase.NullRounds == base.NullRounds {
			log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.TipSet.Cids(), lastBase.NullRounds)
			time.Sleep(time.Duration(build.BlockDelaySecs) * time.Second)
			continue
		}

		// Attempt to mine a block.
		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Errorf("mining block failed: %+v", err)
			time.Sleep(time.Second)
			continue
		}
		lastBase = *base

		// todo figure out this whole bottom section
		//   we won't know if we've mined a block here, we just submit a task
		//   making attempts to mine one

		// Process the mined block.
		if b != nil {
			btime := time.Unix(int64(b.Header.Timestamp), 0)
			now := build.Clock.Now()
			// Handle timing for broadcasting the block.
			switch {
			case btime == now:
				// block timestamp is perfectly aligned with time.
			case btime.After(now):
				// Wait until it's time to broadcast the block.
				if !m.niceSleep(build.Clock.Until(btime)) {
					log.Warnf("received interrupt while waiting to broadcast block, will shutdown after block is sent out")
					build.Clock.Sleep(build.Clock.Until(btime))
				}
			default:
				// Log if the block was mined in the past.
				log.Warnw("mined block in the past",
					"block-time", btime, "time", build.Clock.Now(), "difference", build.Clock.Since(btime))
			}

			// Check for slash filter conditions.
			if os.Getenv("LOTUS_MINER_NO_SLASHFILTER") != "_yes_i_know_i_can_and_probably_will_lose_all_my_fil_and_power_" && !build.IsNearUpgrade(base.TipSet.Height(), build.UpgradeWatermelonFixHeight) {
				witness, fault, err := m.sf.MinedBlock(ctx, b.Header, base.TipSet.Height()+base.NullRounds)
				if err != nil {
					log.Errorf("<!!> SLASH FILTER ERRORED: %s", err)
					// Continue here, because it's _probably_ wiser to not submit this block
					continue
				}

				if fault {
					log.Errorf("<!!> SLASH FILTER DETECTED FAULT due to blocks %s and %s", b.Header.Cid(), witness)
					continue
				}
			}

			// Submit the newly mined block.
			if err := t.api.SyncSubmitBlock(ctx, b); err != nil {
				log.Errorf("failed to submit newly mined block: %+v", err)
			}
		} else {
			// If no block was mined, increase the null rounds and wait for the next epoch.
			base.NullRounds++

			// Calculate the time for the next round.
			nextRound := time.Unix(int64(base.TipSet.MinTimestamp()+build.BlockDelaySecs*uint64(base.NullRounds))+int64(build.PropagationDelaySecs), 0)

			// Wait for the next round.
			time.Sleep(time.Until(nextRound))
		}
	}
}

// GetBestMiningCandidate implements the fork choice rule from a miner's
// perspective.
//
// It obtains the current chain head (HEAD), and compares it to the last tipset
// we selected as our mining base (LAST). If HEAD's weight is larger than
// LAST's weight, it selects HEAD to build on. Else, it selects LAST.
func (t *WinPostTask) GetBestMiningCandidate(ctx context.Context) (*MiningBase, error) {
	bts, err := t.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	if t.lastWork != nil {
		if t.lastWork.TipSet.Equals(bts) {
			return t.lastWork, nil
		}

		btsw, err := t.api.ChainTipSetWeight(ctx, bts.Key())
		if err != nil {
			return nil, err
		}
		ltsw, err := t.api.ChainTipSetWeight(ctx, t.lastWork.TipSet.Key())
		if err != nil {
			t.lastWork = nil
			return nil, err
		}

		if types.BigCmp(btsw, ltsw) <= 0 {
			return t.lastWork, nil
		}
	}

	t.lastWork = &MiningBase{TipSet: bts, ComputeTime: time.Now()}
	return t.lastWork, nil
}

func randTimeOffset(width time.Duration) time.Duration {
	buf := make([]byte, 8)
	rand.Reader.Read(buf) //nolint:errcheck
	val := time.Duration(binary.BigEndian.Uint64(buf) % uint64(width))

	return val - (width / 2)
}

var _ harmonytask.TaskInterface = &WinPostTask{}
