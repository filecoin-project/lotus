package simulation

import (
	"context"
	"math"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
	"golang.org/x/xerrors"
)

func (ss *simulationState) getMinerInfo(ctx context.Context, addr address.Address) (*miner.MinerInfo, error) {
	minerInfo, ok := ss.minerInfos[addr]
	if !ok {
		st, err := ss.stateTree(ctx)
		if err != nil {
			return nil, err
		}
		act, err := st.GetActor(addr)
		if err != nil {
			return nil, err
		}
		minerState, err := miner.Load(ss.Chainstore.ActorStore(ctx), act)
		if err != nil {
			return nil, err
		}
		info, err := minerState.Info()
		if err != nil {
			return nil, err
		}
		minerInfo = &info
		ss.minerInfos[addr] = minerInfo
	}
	return minerInfo, nil
}

func (ss *simulationState) packWindowPoSts(ctx context.Context, cb packFunc) (full bool, _err error) {
	// Push any new window posts into the queue.
	if err := ss.queueWindowPoSts(ctx); err != nil {
		return false, err
	}
	done := 0
	failed := 0
	defer func() {
		if _err != nil {
			return
		}

		log.Debugw("packed window posts",
			"epoch", ss.nextEpoch(),
			"done", done,
			"failed", failed,
			"remaining", len(ss.pendingWposts),
		)
	}()
	// Then pack as many as we can.
	for len(ss.pendingWposts) > 0 {
		next := ss.pendingWposts[0]
		if full, err := cb(next); err != nil {
			if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
				return false, err
			}
			log.Errorw("failed to submit windowed post",
				"error", err,
				"miner", next.To,
				"epoch", ss.nextEpoch(),
			)
			failed++
		} else if full {
			return true, nil
		} else {
			done++
		}

		ss.pendingWposts = ss.pendingWposts[1:]
	}
	ss.pendingWposts = nil
	return false, nil
}

// Enqueue all missing window posts for the current epoch for the given miner.
func (ss *simulationState) stepWindowPoStsMiner(
	ctx context.Context,
	addr address.Address, minerState miner.State,
	commitEpoch abi.ChainEpoch, commitRand abi.Randomness,
) error {

	if active, err := minerState.DeadlineCronActive(); err != nil {
		return err
	} else if !active {
		return nil
	}

	minerInfo, err := ss.getMinerInfo(ctx, addr)
	if err != nil {
		return err
	}

	di, err := minerState.DeadlineInfo(ss.nextEpoch())
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

	var (
		partitions      []miner.PoStPartition
		partitionGroups [][]miner.PoStPartition
	)
	// Only prove partitions with live sectors.
	err = dl.ForEachPartition(func(idx uint64, part miner.Partition) error {
		if proven[idx] {
			return nil
		}
		// TODO: set this to the actual limit from specs-actors.
		// NOTE: We're mimicing the behavior of wdpost_run.go here.
		if len(partitions) > 0 && idx%4 == 0 {
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
			partitions = append(partitions, miner.PoStPartition{Index: idx})
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

	proof, err := mockWpostProof(minerInfo.WindowPoStProofType, addr)
	if err != nil {
		return err
	}
	for _, group := range partitionGroups {
		params := miner.SubmitWindowedPoStParams{
			Deadline:   di.Index,
			Partitions: group,
			Proofs: []proof5.PoStProof{{
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
			Method: miner.Methods.SubmitWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
		}
		ss.pendingWposts = append(ss.pendingWposts, msg)
	}
	return nil
}

// Enqueue missing window posts for all miners with deadlines opening at the current epoch.
func (ss *simulationState) queueWindowPoSts(ctx context.Context) error {
	targetHeight := ss.nextEpoch()

	st, err := ss.stateTree(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	was := len(ss.pendingWposts)
	count := 0
	defer func() {
		log.Debugw("computed window posts",
			"miners", count,
			"count", len(ss.pendingWposts)-was,
			"duration", time.Since(now),
		)
	}()

	// Perform a bit of catch up. This lets us do things like skip blocks at upgrades then catch
	// up to make the simualtion easier.
	for ; ss.nextWpostEpoch <= targetHeight; ss.nextWpostEpoch++ {
		if ss.nextWpostEpoch+miner.WPoStChallengeWindow < targetHeight {
			log.Warnw("skipping old window post", "epoch", ss.nextWpostEpoch)
			continue
		}
		commitEpoch := ss.nextWpostEpoch - 1
		commitRand, err := ss.postChainCommitInfo(ctx, commitEpoch)
		if err != nil {
			return err
		}

		store := ss.Chainstore.ActorStore(ctx)

		for _, addr := range ss.wpostPeriods[int(ss.nextWpostEpoch%miner.WPoStChallengeWindow)] {
			minerActor, err := st.GetActor(addr)
			if err != nil {
				return err
			}
			minerState, err := miner.Load(store, minerActor)
			if err != nil {
				return err
			}
			if err := ss.stepWindowPoStsMiner(ctx, addr, minerState, commitEpoch, commitRand); err != nil {
				return err
			}
			count++
		}

	}
	return nil
}
