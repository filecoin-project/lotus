package stages

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	power5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/power"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/mock"
)

const (
	minProveCommitBatchSize = 4
	maxProveCommitBatchSize = miner5.MaxAggregatedSectors
)

type ProveCommitStage struct {
	funding Funding
	// We track the set of pending commits. On simulation load, and when a new pre-commit is
	// added to the chain, we put the commit in this queue. advanceEpoch(currentEpoch) should be
	// called on this queue at every epoch before using it.
	commitQueue commitQueue
	initialized bool
}

func NewProveCommitStage(funding Funding) (*ProveCommitStage, error) {
	return &ProveCommitStage{
		funding: funding,
	}, nil
}

func (*ProveCommitStage) Name() string {
	return "prove-commit"
}

func (stage *ProveCommitStage) EnqueueProveCommit(
	minerAddr address.Address, preCommitEpoch abi.ChainEpoch, info minertypes.SectorPreCommitInfo,
) error {
	return stage.commitQueue.enqueueProveCommit(minerAddr, preCommitEpoch, info)
}

// PackMessages packs all prove-commits for all "ready to be proven" sectors until it fills the
// block or runs out.
func (stage *ProveCommitStage) PackMessages(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	if !stage.initialized {
		if err := stage.load(ctx, bb); err != nil {
			return err
		}
	}
	// Roll the commitQueue forward.
	stage.commitQueue.advanceEpoch(bb.Height())

	start := time.Now()
	var failed, done, unbatched, count int
	defer func() {
		if _err != nil {
			return
		}
		remaining := stage.commitQueue.ready()
		bb.L().Debugw("packed prove commits",
			"remaining", remaining,
			"done", done,
			"failed", failed,
			"unbatched", unbatched,
			"miners-processed", count,
			"duration", time.Since(start),
		)
	}()

	for {
		addr, pending, ok := stage.commitQueue.nextMiner()
		if !ok {
			return nil
		}

		res, err := stage.packProveCommitsMiner(ctx, bb, addr, pending)
		if err != nil {
			return err
		}
		failed += res.failed
		done += res.done
		unbatched += res.unbatched
		count++
		if res.full {
			return nil
		}
	}
}

type proveCommitResult struct {
	done, failed, unbatched int
	full                    bool
}

// packProveCommitsMiner enqueues a prove commits from the given miner until it runs out of
// available prove-commits, batching as much as possible.
//
// This function will fund as necessary from the "burnt funds actor" (look, it's convenient).
func (stage *ProveCommitStage) packProveCommitsMiner(
	ctx context.Context, bb *blockbuilder.BlockBuilder, minerAddr address.Address,
	pending minerPendingCommits,
) (res proveCommitResult, _err error) {
	minerActor, err := bb.StateTree().GetActor(minerAddr)
	if err != nil {
		return res, err
	}
	minerState, err := miner.Load(bb.ActorStore(), minerActor)
	if err != nil {
		return res, err
	}
	info, err := minerState.Info()
	if err != nil {
		return res, err
	}

	log := bb.L().With("miner", minerAddr)

	nv := bb.NetworkVersion()
	for sealType, snos := range pending {
		if nv >= network.Version13 {
			for len(snos) > minProveCommitBatchSize {
				batchSize := maxProveCommitBatchSize
				if len(snos) < batchSize {
					batchSize = len(snos)
				}
				batch := snos[:batchSize]

				proof, err := mock.MockAggregateSealProof(sealType, minerAddr, batchSize)
				if err != nil {
					return res, err
				}

				params := miner5.ProveCommitAggregateParams{
					SectorNumbers:  bitfield.New(),
					AggregateProof: proof,
				}
				for _, sno := range batch {
					params.SectorNumbers.Set(uint64(sno))
				}

				enc, err := actors.SerializeParams(&params)
				if err != nil {
					return res, err
				}

				if _, err := stage.funding.SendAndFund(bb, &types.Message{
					From:   info.Worker,
					To:     minerAddr,
					Value:  abi.NewTokenAmount(0),
					Method: builtin.MethodsMiner.ProveCommitAggregate,
					Params: enc,
				}); err == nil {
					res.done += len(batch)
				} else if blockbuilder.IsOutOfGas(err) {
					res.full = true
					return res, nil
				} else if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
					// If we get a random error, or a fatal actor error, bail.
					return res, err
				} else if aerr.RetCode() == exitcode.ErrNotFound || aerr.RetCode() == exitcode.ErrIllegalArgument {
					// If we get a "not-found" or illegal argument error, try to
					// remove any missing prove-commits and continue. This can
					// happen either because:
					//
					// 1. The pre-commit failed on execution (but not when
					// packing). This shouldn't happen, but we might as well
					// gracefully handle it.
					// 2. The pre-commit has expired. We'd have to be really
					// backloged to hit this case, but we might as well handle
					// it.
					// First, split into "good" and "missing"
					good, err := stage.filterProveCommits(ctx, bb, minerAddr, batch)
					if err != nil {
						log.Errorw("failed to filter prove commits", "error", err)
						// fail with the original error.
						return res, aerr
					}
					removed := len(batch) - len(good)
					if removed == 0 {
						log.Errorw("failed to prove-commit for unknown reasons",
							"error", aerr,
							"sectors", batch,
						)
						res.failed += len(batch)
					} else if len(good) == 0 {
						log.Errorw("failed to prove commit missing pre-commits",
							"error", aerr,
							"discarded", removed,
						)
						res.failed += len(batch)
					} else {
						// update the pending sector numbers in-place to remove the expired ones.
						snos = snos[removed:]
						copy(snos, good)
						pending.finish(sealType, removed)

						log.Errorw("failed to prove commit expired/missing pre-commits",
							"error", aerr,
							"discarded", removed,
							"kept", len(good),
						)
						res.failed += removed

						// Then try again.
						continue
					}
				} else {
					log.Errorw("failed to prove commit sector(s)",
						"error", err,
						"sectors", batch,
					)
					res.failed += len(batch)
				}
				pending.finish(sealType, len(batch))
				snos = snos[len(batch):]
			}
		}
		for len(snos) > 0 && res.unbatched < power5.MaxMinerProveCommitsPerEpoch {
			sno := snos[0]
			snos = snos[1:]

			proof, err := mock.MockSealProof(sealType, minerAddr)
			if err != nil {
				return res, err
			}
			params := minertypes.ProveCommitSectorParams{
				SectorNumber: sno,
				Proof:        proof,
			}
			enc, err := actors.SerializeParams(&params)
			if err != nil {
				return res, err
			}
			if _, err := stage.funding.SendAndFund(bb, &types.Message{
				From:   info.Worker,
				To:     minerAddr,
				Value:  abi.NewTokenAmount(0),
				Method: builtin.MethodsMiner.ProveCommitSector,
				Params: enc,
			}); err == nil {
				res.unbatched++
				res.done++
			} else if blockbuilder.IsOutOfGas(err) {
				res.full = true
				return res, nil
			} else if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
				return res, err
			} else {
				log.Errorw("failed to prove commit sector(s)",
					"error", err,
					"sectors", []abi.SectorNumber{sno},
				)
				res.failed++
			}
			// mark it as "finished" regardless so we skip it.
			pending.finish(sealType, 1)
		}
		// if we get here, we can't pre-commit anything more.
	}
	return res, nil
}

// loadMiner enqueue all pending prove-commits for the given miner. This is called on load to
// populate the commitQueue and should not need to be called later.
//
// It will drop any pre-commits that have already expired.
func (stage *ProveCommitStage) loadMiner(ctx context.Context, bb *blockbuilder.BlockBuilder, addr address.Address) error {
	epoch := bb.Height()
	av, err := bb.ActorsVersion()
	if err != nil {
		return err
	}

	minerState, err := loadMiner(bb.ActorStore(), bb.ParentStateTree(), addr)
	if err != nil {
		return err
	}

	// Find all pending prove commits and group by proof type. Really, there should never
	// (except during upgrades be more than one type.
	var total, dropped int
	err = minerState.ForEachPrecommittedSector(func(info minertypes.SectorPreCommitOnChainInfo) error {
		total++
		msd, err := policy.GetMaxProveCommitDuration(av, info.Info.SealProof)
		if err != nil {
			return err
		}
		if epoch > info.PreCommitEpoch+msd {
			dropped++
			return nil
		}
		return stage.commitQueue.enqueueProveCommit(addr, info.PreCommitEpoch, info.Info)
	})
	if err != nil {
		return err
	}
	if dropped > 0 {
		bb.L().Warnw("dropped expired pre-commits on load",
			"miner", addr,
			"total", total,
			"expired", dropped,
		)
	}
	return nil
}

// filterProveCommits filters out expired and/or missing pre-commits.
func (stage *ProveCommitStage) filterProveCommits(
	ctx context.Context, bb *blockbuilder.BlockBuilder,
	minerAddr address.Address, snos []abi.SectorNumber,
) ([]abi.SectorNumber, error) {
	act, err := bb.StateTree().GetActor(minerAddr)
	if err != nil {
		return nil, err
	}

	minerState, err := miner.Load(bb.ActorStore(), act)
	if err != nil {
		return nil, err
	}

	nextEpoch := bb.Height()
	av, err := bb.ActorsVersion()
	if err != nil {
		return nil, err
	}

	good := make([]abi.SectorNumber, 0, len(snos))
	for _, sno := range snos {
		info, err := minerState.GetPrecommittedSector(sno)
		if err != nil {
			return nil, err
		}
		if info == nil {
			continue
		}
		msd, err := policy.GetMaxProveCommitDuration(av, info.Info.SealProof)
		if err != nil {
			return nil, err
		}
		if nextEpoch > info.PreCommitEpoch+msd {
			continue
		}
		good = append(good, sno)
	}
	return good, nil
}

func (stage *ProveCommitStage) load(ctx context.Context, bb *blockbuilder.BlockBuilder) error {
	stage.initialized = false // in case something fails while we're doing this.
	stage.commitQueue = commitQueue{offset: bb.Height()}
	powerState, err := loadPower(bb.ActorStore(), bb.ParentStateTree())
	if err != nil {
		return err
	}

	err = powerState.ForEachClaim(func(minerAddr address.Address, claim power.Claim) error {
		// TODO: If we want to finish pre-commits for "new" miners, we'll need to change
		// this.
		if claim.RawBytePower.IsZero() {
			return nil
		}
		return stage.loadMiner(ctx, bb, minerAddr)
	}, false)
	if err != nil {
		return err
	}

	stage.initialized = true
	return nil
}
