package simulation

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	power5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/power"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
)

// packProveCommits packs all prove-commits for all "ready to be proven" sectors until it fills the
// block or runs out.
func (ss *simulationState) packProveCommits(ctx context.Context, cb packFunc) (_err error) {
	// Roll the commitQueue forward.
	ss.commitQueue.advanceEpoch(ss.nextEpoch())

	start := time.Now()
	var full, failed, done, unbatched, count int
	defer func() {
		if _err != nil {
			return
		}
		remaining := ss.commitQueue.ready()
		log.Debugw("packed prove commits",
			"remaining", remaining,
			"done", done,
			"failed", failed,
			"unbatched", unbatched,
			"miners-processed", count,
			"filled-block", full,
			"duration", time.Since(start),
		)
	}()

	for {
		addr, pending, ok := ss.commitQueue.nextMiner()
		if !ok {
			return nil
		}

		res, err := ss.packProveCommitsMiner(ctx, cb, addr, pending)
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
func (ss *simulationState) packProveCommitsMiner(
	ctx context.Context, cb packFunc, minerAddr address.Address,
	pending minerPendingCommits,
) (res proveCommitResult, _err error) {
	info, err := ss.getMinerInfo(ctx, minerAddr)
	if err != nil {
		return res, err
	}

	nv := ss.StateManager.GetNtwkVersion(ctx, ss.nextEpoch())
	for sealType, snos := range pending {
		if nv >= network.Version13 {
			for len(snos) > minProveCommitBatchSize {
				batchSize := maxProveCommitBatchSize
				if len(snos) < batchSize {
					batchSize = len(snos)
				}
				batch := snos[:batchSize]

				proof, err := mockAggregateSealProof(sealType, minerAddr, batchSize)
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

				if _, err := sendAndFund(cb, &types.Message{
					From:   info.Worker,
					To:     minerAddr,
					Value:  abi.NewTokenAmount(0),
					Method: miner.Methods.ProveCommitAggregate,
					Params: enc,
				}); err == nil {
					res.done += len(batch)
				} else if err == ErrOutOfGas {
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
					good, err := ss.filterProveCommits(ctx, minerAddr, batch)
					if err != nil {
						log.Errorw("failed to filter prove commits", "miner", minerAddr, "error", err)
						// fail with the original error.
						return res, aerr
					}
					removed := len(batch) - len(good)
					if removed == 0 {
						log.Errorw("failed to prove-commit for unknown reasons",
							"error", aerr,
							"miner", minerAddr,
							"sectors", batch,
							"epoch", ss.nextEpoch(),
						)
						res.failed += len(batch)
					} else if len(good) == 0 {
						log.Errorw("failed to prove commit missing pre-commits",
							"error", aerr,
							"miner", minerAddr,
							"discarded", removed,
							"epoch", ss.nextEpoch(),
						)
						res.failed += len(batch)
					} else {
						// update the pending sector numbers in-place to remove the expired ones.
						snos = snos[removed:]
						copy(snos, good)
						pending.finish(sealType, removed)

						log.Errorw("failed to prove commit expired/missing pre-commits",
							"error", aerr,
							"miner", minerAddr,
							"discarded", removed,
							"kept", len(good),
							"epoch", ss.nextEpoch(),
						)
						res.failed += removed

						// Then try again.
						continue
					}
					log.Errorw("failed to prove commit missing sector(s)",
						"error", err,
						"miner", minerAddr,
						"sectors", batch,
						"epoch", ss.nextEpoch(),
					)
					res.failed += len(batch)
				} else {
					log.Errorw("failed to prove commit sector(s)",
						"error", err,
						"miner", minerAddr,
						"sectors", batch,
						"epoch", ss.nextEpoch(),
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

			proof, err := mockSealProof(sealType, minerAddr)
			if err != nil {
				return res, err
			}
			params := miner.ProveCommitSectorParams{
				SectorNumber: sno,
				Proof:        proof,
			}
			enc, err := actors.SerializeParams(&params)
			if err != nil {
				return res, err
			}
			if _, err := sendAndFund(cb, &types.Message{
				From:   info.Worker,
				To:     minerAddr,
				Value:  abi.NewTokenAmount(0),
				Method: miner.Methods.ProveCommitSector,
				Params: enc,
			}); err == nil {
				res.unbatched++
				res.done++
			} else if err == ErrOutOfGas {
				res.full = true
				return res, nil
			} else if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
				return res, err
			} else {
				log.Errorw("failed to prove commit sector(s)",
					"error", err,
					"miner", minerAddr,
					"sectors", []abi.SectorNumber{sno},
					"epoch", ss.nextEpoch(),
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

// loadProveCommitsMiner enqueue all pending prove-commits for the given miner. This is called on
// load to populate the commitQueue and should not need to be called later.
//
// It will drop any pre-commits that have already expired.
func (ss *simulationState) loadProveCommitsMiner(ctx context.Context, addr address.Address, minerState miner.State) error {
	// Find all pending prove commits and group by proof type. Really, there should never
	// (except during upgrades be more than one type.
	nextEpoch := ss.nextEpoch()
	nv := ss.StateManager.GetNtwkVersion(ctx, nextEpoch)
	av := actors.VersionForNetwork(nv)

	var total, dropped int
	err := minerState.ForEachPrecommittedSector(func(info miner.SectorPreCommitOnChainInfo) error {
		total++
		msd := policy.GetMaxProveCommitDuration(av, info.Info.SealProof)
		if nextEpoch > info.PreCommitEpoch+msd {
			dropped++
			return nil
		}
		return ss.commitQueue.enqueueProveCommit(addr, info.PreCommitEpoch, info.Info)
	})
	if err != nil {
		return err
	}
	if dropped > 0 {
		log.Warnw("dropped expired pre-commits on load",
			"miner", addr,
			"total", total,
			"expired", dropped,
		)
	}
	return nil
}

// filterProveCommits filters out expired and/or missing pre-commits.
func (ss *simulationState) filterProveCommits(ctx context.Context, minerAddr address.Address, snos []abi.SectorNumber) ([]abi.SectorNumber, error) {
	_, minerState, err := ss.getMinerState(ctx, minerAddr)
	if err != nil {
		return nil, err
	}

	nextEpoch := ss.nextEpoch()
	nv := ss.StateManager.GetNtwkVersion(ctx, nextEpoch)
	av := actors.VersionForNetwork(nv)

	good := make([]abi.SectorNumber, 0, len(snos))
	for _, sno := range snos {
		info, err := minerState.GetPrecommittedSector(sno)
		if err != nil {
			return nil, err
		}
		if info == nil {
			continue
		}
		msd := policy.GetMaxProveCommitDuration(av, info.Info.SealProof)
		if nextEpoch > info.PreCommitEpoch+msd {
			continue
		}
		good = append(good, sno)
	}
	return good, nil
}
