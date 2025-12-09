package stages

import (
	"context"
	"math"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/mock"
)

type WindowPoStStage struct {
	// We track the window post periods per miner and assume that no new miners are ever added.

	// We record all pending window post messages, and the epoch up through which we've
	// generated window post messages.
	pendingWposts  []*types.Message
	wpostPeriods   [][]address.Address // (epoch % (epochs in a deadline)) -> miner
	nextWpostEpoch abi.ChainEpoch
}

func NewWindowPoStStage() (*WindowPoStStage, error) {
	return new(WindowPoStStage), nil
}

func (*WindowPoStStage) Name() string {
	return "window-post"
}

// PackMessages packs window posts until either the block is full or all healthy sectors
// have been proven. It does not recover sectors.
func (stage *WindowPoStStage) PackMessages(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	// Push any new window posts into the queue.
	if err := stage.tick(ctx, bb); err != nil {
		return err
	}
	done := 0
	failed := 0
	defer func() {
		if _err != nil {
			return
		}

		bb.L().Debugw("packed window posts",
			"done", done,
			"failed", failed,
			"remaining", len(stage.pendingWposts),
		)
	}()
	// Then pack as many as we can.
	for len(stage.pendingWposts) > 0 {
		next := stage.pendingWposts[0]
		if _, err := bb.PushMessage(next); err != nil {
			if blockbuilder.IsOutOfGas(err) {
				return nil
			}
			if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
				return err
			}
			bb.L().Errorw("failed to submit windowed post",
				"error", err,
				"miner", next.To,
			)
			failed++
		} else {
			done++
		}

		stage.pendingWposts = stage.pendingWposts[1:]
	}
	stage.pendingWposts = nil
	return nil
}

// queueMiner enqueues all missing window posts for the current epoch for the given miner.
func (stage *WindowPoStStage) queueMiner(
	ctx context.Context, bb *blockbuilder.BlockBuilder,
	addr address.Address, minerState miner.State,
	commitEpoch abi.ChainEpoch, commitRand abi.Randomness,
) error {

	if active, err := minerState.DeadlineCronActive(); err != nil {
		return err
	} else if !active {
		return nil
	}

	minerInfo, err := minerState.Info()
	if err != nil {
		return err
	}

	di, err := minerState.DeadlineInfo(bb.Height())
	if err != nil {
		return err
	}
	di = di.NextNotElapsed()

	dl, err := minerState.LoadDeadline(di.Index)
	if err != nil {
		return err
	}

	provenBf, err := dl.PartitionsPoSted()
	if err != nil {
		return err
	}
	proven, err := provenBf.AllMap(math.MaxUint64)
	if err != nil {
		return err
	}

	poStBatchSize, err := policy.GetMaxPoStPartitions(bb.NetworkVersion(), minerInfo.WindowPoStProofType)
	if err != nil {
		return err
	}

	var (
		partitions      []minertypes.PoStPartition
		partitionGroups [][]minertypes.PoStPartition
	)
	// Only prove partitions with live sectors.
	err = dl.ForEachPartition(func(idx uint64, part miner.Partition) error {
		if proven[idx] {
			return nil
		}
		// NOTE: We're mimicking the behavior of wdpost_run.go here.
		if len(partitions) > 0 && idx%uint64(poStBatchSize) == 0 {
			partitionGroups = append(partitionGroups, partitions)
			partitions = nil

		}
		live, err := part.LiveSectors()
		if err != nil {
			return err
		}
		liveCount, err := live.Count()
		if err != nil {
			return err
		}
		faulty, err := part.FaultySectors()
		if err != nil {
			return err
		}
		faultyCount, err := faulty.Count()
		if err != nil {
			return err
		}
		if liveCount-faultyCount > 0 {
			partitions = append(partitions, minertypes.PoStPartition{Index: idx})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(partitions) > 0 {
		partitionGroups = append(partitionGroups, partitions)
		partitions = nil
	}

	proof, err := mock.MockWindowPoStProof(minerInfo.WindowPoStProofType, addr)
	if err != nil {
		return err
	}
	for _, group := range partitionGroups {
		params := minertypes.SubmitWindowedPoStParams{
			Deadline:   di.Index,
			Partitions: group,
			Proofs: []prooftypes.PoStProof{{
				PoStProof:  minerInfo.WindowPoStProofType,
				ProofBytes: proof,
			}},
			ChainCommitEpoch: commitEpoch,
			ChainCommitRand:  commitRand,
		}
		enc, aerr := actors.SerializeParams(&params)
		if aerr != nil {
			return xerrors.Errorf("could not serialize submit window post parameters: %w", aerr)
		}
		msg := &types.Message{
			To:     addr,
			From:   minerInfo.Worker,
			Method: builtin.MethodsMiner.SubmitWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
		}
		stage.pendingWposts = append(stage.pendingWposts, msg)
	}
	return nil
}

func (stage *WindowPoStStage) load(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	bb.L().Info("loading window post info")

	start := time.Now()
	defer func() {
		if _err != nil {
			return
		}

		bb.L().Infow("loaded window post info", "duration", time.Since(start))
	}()

	// reset
	stage.wpostPeriods = make([][]address.Address, minertypes.WPoStChallengeWindow)
	stage.pendingWposts = nil
	stage.nextWpostEpoch = bb.Height() + 1

	st := bb.ParentStateTree()
	store := bb.ActorStore()

	powerState, err := loadPower(store, st)
	if err != nil {
		return err
	}

	commitEpoch := bb.ParentTipSet().Height()
	commitRand, err := postChainCommitInfo(ctx, bb, commitEpoch)
	if err != nil {
		return err
	}

	return powerState.ForEachClaim(func(minerAddr address.Address, claim power.Claim) error {
		// TODO: If we start recovering power, we'll need to change this.
		if claim.RawBytePower.IsZero() {
			return nil
		}

		minerState, err := loadMiner(store, st, minerAddr)
		if err != nil {
			return err
		}

		// Shouldn't be necessary if the miner has power, but we might as well be safe.
		if active, err := minerState.DeadlineCronActive(); err != nil {
			return err
		} else if !active {
			return nil
		}

		// Record when we need to prove for this miner.
		dinfo, err := minerState.DeadlineInfo(bb.Height())
		if err != nil {
			return err
		}
		dinfo = dinfo.NextNotElapsed()

		ppOffset := int(dinfo.PeriodStart % minertypes.WPoStChallengeWindow)
		stage.wpostPeriods[ppOffset] = append(stage.wpostPeriods[ppOffset], minerAddr)

		return stage.queueMiner(ctx, bb, minerAddr, minerState, commitEpoch, commitRand)
	}, false)
}

func (stage *WindowPoStStage) tick(ctx context.Context, bb *blockbuilder.BlockBuilder) error {
	// If this is our first time, load from scratch.
	if stage.wpostPeriods == nil {
		return stage.load(ctx, bb)
	}

	targetHeight := bb.Height()
	now := time.Now()
	was := len(stage.pendingWposts)
	count := 0
	defer func() {
		bb.L().Debugw("computed window posts",
			"miners", count,
			"count", len(stage.pendingWposts)-was,
			"duration", time.Since(now),
		)
	}()

	st := bb.ParentStateTree()
	store := bb.ActorStore()

	// Perform a bit of catch up. This lets us do things like skip blocks at upgrades then catch
	// up to make the simulation easier.
	for ; stage.nextWpostEpoch <= targetHeight; stage.nextWpostEpoch++ {
		if stage.nextWpostEpoch+minertypes.WPoStChallengeWindow < targetHeight {
			bb.L().Warnw("skipping old window post", "deadline-open", stage.nextWpostEpoch)
			continue
		}
		commitEpoch := stage.nextWpostEpoch - 1
		commitRand, err := postChainCommitInfo(ctx, bb, commitEpoch)
		if err != nil {
			return err
		}

		for _, addr := range stage.wpostPeriods[int(stage.nextWpostEpoch%minertypes.WPoStChallengeWindow)] {
			minerState, err := loadMiner(store, st, addr)
			if err != nil {
				return err
			}

			if err := stage.queueMiner(ctx, bb, addr, minerState, commitEpoch, commitRand); err != nil {
				return err
			}
			count++
		}

	}
	return nil
}
