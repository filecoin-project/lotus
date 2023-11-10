package lpwinning

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"time"
)

var log = logging.Logger("lpwinning")

type WinPostTask struct {
	max abi.SectorNumber

	// lastWork holds the last MiningBase we built upon.
	lastWork *MiningBase

	api    WinPostAPI
	actors []dtypes.MinerAddress

	mineTF promise.Promise[harmonytask.AddTaskFunc]
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
	t.mineTF.Set(taskFunc)
}

// MiningBase is the tipset on top of which we plan to construct our next block.
// Refer to godocs on GetBestMiningCandidate.
type MiningBase struct {
	TipSet      *types.TipSet
	ComputeTime time.Time
	AddRounds   abi.ChainEpoch
}

func (mb MiningBase) epoch() abi.ChainEpoch {
	// return the epoch that will result from mining on this base
	return mb.TipSet.Height() + mb.AddRounds + 1
}

func (mb MiningBase) baseTime() time.Time {
	tsTime := time.Unix(int64(mb.TipSet.MinTimestamp()), 0)
	roundDelay := build.BlockDelaySecs * uint64(mb.AddRounds+1)
	tsTime = tsTime.Add(time.Duration(roundDelay) * time.Second)
	return tsTime
}

func (mb MiningBase) afterPropDelay() time.Time {
	base := mb.baseTime()
	base.Add(randTimeOffset(time.Second))
	return base
}

func retry1[R any](f func() (R, error)) R {
	for {
		r, err := f()
		if err == nil {
			return r
		}

		log.Errorw("error in mining loop, retrying", "error", err)
		time.Sleep(time.Second)
	}
}

func (t *WinPostTask) mineBasic(ctx context.Context) {
	var workBase MiningBase

	taskFn := t.mineTF.Val(ctx)

	{
		head := retry1(func() (*types.TipSet, error) {
			return t.api.ChainHead(ctx)
		})

		workBase = MiningBase{
			TipSet:      head,
			AddRounds:   0,
			ComputeTime: time.Now(),
		}
	}

	/*

		         /- T+0 == workBase.baseTime
		         |
		>--------*------*--------[wait until next round]----->
		                |
		                |- T+PD == workBase.afterPropDelay+(~1s)
		                |- Here we acquire the new workBase, and start a new round task
		                \- Then we loop around, and wait for the next head

		time -->
	*/

	for {
		// wait for *NEXT* propagation delay
		time.Sleep(time.Until(workBase.afterPropDelay()))

		// check current best candidate
		maybeBase := retry1(func() (*types.TipSet, error) {
			return t.api.ChainHead(ctx)
		})

		if workBase.TipSet.Equals(maybeBase) {
			// workbase didn't change in the new round so we have a null round here
			workBase.AddRounds++
			log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "same-tipset")
		} else {
			btsw := retry1(func() (types.BigInt, error) {
				return t.api.ChainTipSetWeight(ctx, maybeBase.Key())
			})

			ltsw := retry1(func() (types.BigInt, error) {
				return t.api.ChainTipSetWeight(ctx, workBase.TipSet.Key())
			})

			if types.BigCmp(btsw, ltsw) <= 0 {
				// new tipset for some reason has less weight than the old one, assume null round here
				// NOTE: the backing node may have reorged, or manually changed head
				workBase.AddRounds++
				log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "prefer-local-weight")
			} else {
				// new tipset has more weight, so we should mine on it, no null round here
				log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "prefer-new-tipset")

				workBase = MiningBase{
					TipSet:      maybeBase,
					AddRounds:   0,
					ComputeTime: time.Now(),
				}
			}
		}

		// dispatch mining task
		// (note equivocation prevention is handled by the mining code)

		for _, act := range t.actors {
			spID, err := address.IDFromAddress(address.Address(act))
			if err != nil {
				log.Errorf("failed to get spID from address %s: %s", act, err)
				continue
			}

			taskFn(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
				_, err := tx.Exec(`INSERT INTO mining_tasks (task_id, sp_id, epoch) VALUES ($1, $2, $3)`, id, spID, workBase.epoch())
				if err != nil {
					return false, xerrors.Errorf("inserting mining_tasks: %w", err)
				}

				for _, c := range workBase.TipSet.Cids() {
					_, err = tx.Exec(`INSERT INTO mining_base_block (task_id, block_cid) VALUES ($1, $2)`, id, c)
					if err != nil {
						return false, xerrors.Errorf("inserting mining base blocks: %w", err)
					}
				}

				return true, nil // no errors, commit the transaction
			})
		}
	}
}

/*
	func (t *WinPostTask) mine2(ctx context.Context) {
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
				if base != nil && base.TipSet.Height() == prebase.TipSet.Height() && base.AddRounds == prebase.AddRounds {
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
				_, err = t.api.StateGetBeaconEntry(ctx, prebase.TipSet.Height()+prebase.AddRounds+1)
				if err != nil {
					log.Errorf("failed getting beacon entry: %s", err)
					time.Sleep(time.Second)
					continue
				}

				base = prebase
			}

			// Check for repeated mining candidates and handle sleep for the next round.
			if base.TipSet.Equals(lastBase.TipSet) && lastBase.AddRounds == base.AddRounds {
				log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.TipSet.Cids(), lastBase.AddRounds)
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
					witness, fault, err := m.sf.MinedBlock(ctx, b.Header, base.TipSet.Height()+base.AddRounds)
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
				base.AddRounds++

				// Calculate the time for the next round.
				nextRound := time.Unix(int64(base.TipSet.MinTimestamp()+build.BlockDelaySecs*uint64(base.AddRounds))+int64(build.PropagationDelaySecs), 0)

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
*/
func randTimeOffset(width time.Duration) time.Duration {
	buf := make([]byte, 8)
	rand.Reader.Read(buf) //nolint:errcheck
	val := time.Duration(binary.BigEndian.Uint64(buf) % uint64(width))

	return val - (width / 2)
}

var _ harmonytask.TaskInterface = &WinPostTask{}
