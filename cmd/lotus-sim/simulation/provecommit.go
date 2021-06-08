package simulation

import (
	"context"

	"github.com/filecoin-project/go-bitfield"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"

	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	power5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/power"
)

// packProveCOmmits packs all prove-commits for all "ready to be proven" sectors until it fills the
// block or runs out.
func (ss *simulationState) packProveCommits(ctx context.Context, cb packFunc) (_full bool, _err error) {
	// Roll the commitQueue forward.
	ss.commitQueue.advanceEpoch(ss.nextEpoch())

	var failed, done, unbatched, count int
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
			"filled-block", _full,
		)
	}()

	for {
		addr, pending, ok := ss.commitQueue.nextMiner()
		if !ok {
			return false, nil
		}

		res, full, err := ss.packProveCommitsMiner(ctx, cb, addr, pending)
		if err != nil {
			return false, err
		}
		failed += res.failed
		done += res.done
		unbatched += res.unbatched
		count++
		if full {
			return true, nil
		}
	}
}

type proveCommitResult struct {
	done, failed, unbatched int
}

// sendAndFund "packs" the given message, funding the actor if necessary. It:
//
// 1. Tries to send the given message.
// 2. If that fails, it checks to see if the exit code was ErrInsufficientFunds.
// 3. If so, it sends 1K FIL from the "burnt funds actor" (because we need to send it from
//    somewhere) and re-tries the message.0
//
// NOTE: If the message fails a second time, the funds won't be "unsent".
func sendAndFund(send packFunc, msg *types.Message) (bool, error) {
	full, err := send(msg)
	aerr, ok := err.(aerrors.ActorError)
	if !ok || aerr.RetCode() != exitcode.ErrInsufficientFunds {
		return full, err
	}
	// Ok, insufficient funds. Let's fund this miner and try again.
	full, err = send(&types.Message{
		From:   builtin.BurntFundsActorAddr,
		To:     msg.To,
		Value:  targetFunds,
		Method: builtin.MethodSend,
	})
	if err != nil {
		return false, xerrors.Errorf("failed to fund %s: %w", msg.To, err)
	}
	// ok, nothing's going to work.
	if full {
		return true, nil
	}
	return send(msg)
}

// packProveCommitsMiner enqueues a prove commits from the given miner until it runs out of
// available prove-commits, batching as much as possible.
//
// This function will fund as necessary from the "burnt funds actor" (look, it's convenient).
func (ss *simulationState) packProveCommitsMiner(
	ctx context.Context, cb packFunc, minerAddr address.Address,
	pending minerPendingCommits,
) (res proveCommitResult, full bool, _err error) {
	info, err := ss.getMinerInfo(ctx, minerAddr)
	if err != nil {
		return res, false, err
	}

	nv := ss.sm.GetNtwkVersion(ctx, ss.nextEpoch())
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
					return res, false, err
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
					return res, false, err
				}

				if full, err := sendAndFund(cb, &types.Message{
					From:   info.Worker,
					To:     minerAddr,
					Value:  abi.NewTokenAmount(0),
					Method: miner.Methods.ProveCommitAggregate,
					Params: enc,
				}); err != nil {
					// If we get a random error, or a fatal actor error, bail.
					// Otherwise, just log it.
					aerr, ok := err.(aerrors.ActorError)
					if !ok || aerr.IsFatal() {
						return res, false, err
					}
					if aerr.RetCode() == exitcode.ErrNotFound {
						good, expired, err := ss.filterProveCommits(ctx, minerAddr, batch)
						if err != nil {
							log.Errorw("failed to filter prove commits", "miner", minerAddr, "error", err)
							// fail with the original error.
							return res, false, aerr
						}
						// If we've removed sectors (and kept some), try again.
						// If we've removed all sectors, or no sectors, just
						// move on and deliver the error.
						if len(good) > 0 && len(expired) > 0 {
							res.failed += len(expired)

							// update the pending sector numbers in-place to remove the expired ones.
							snos = snos[len(expired):]
							copy(snos, good)
							pending.finish(sealType, len(expired))

							log.Errorw("failed to prove commit expired/missing pre-commits",
								"error", err,
								"miner", minerAddr,
								"expired", expired,
								"discarded", len(expired),
								"kept", len(good),
								"epoch", ss.nextEpoch(),
							)
							continue
						}
					}
					log.Errorw("failed to prove commit sector(s)",
						"error", err,
						"miner", minerAddr,
						"sectors", batch,
						"epoch", ss.nextEpoch(),
					)
					res.failed += len(batch)
				} else if full {
					return res, true, nil
				} else {
					res.done += len(batch)
				}
				pending.finish(sealType, batchSize)
				snos = snos[batchSize:]
			}
		}
		for len(snos) > 0 && res.unbatched < power5.MaxMinerProveCommitsPerEpoch {
			sno := snos[0]
			snos = snos[1:]

			proof, err := mockSealProof(sealType, minerAddr)
			if err != nil {
				return res, false, err
			}
			params := miner.ProveCommitSectorParams{
				SectorNumber: sno,
				Proof:        proof,
			}
			enc, err := actors.SerializeParams(&params)
			if err != nil {
				return res, false, err
			}
			if full, err := sendAndFund(cb, &types.Message{
				From:   info.Worker,
				To:     minerAddr,
				Value:  abi.NewTokenAmount(0),
				Method: miner.Methods.ProveCommitSector,
				Params: enc,
			}); err != nil {
				if aerr, ok := err.(aerrors.ActorError); !ok || aerr.IsFatal() {
					return res, false, err
				}
				log.Errorw("failed to prove commit sector(s)",
					"error", err,
					"miner", minerAddr,
					"sectors", []abi.SectorNumber{sno},
					"epoch", ss.nextEpoch(),
				)
				res.failed++
			} else if full {
				return res, true, nil
			} else {
				res.unbatched++
				res.done++
			}
			// mark it as "finished" regardless so we skip it.
			pending.finish(sealType, 1)
		}
		// if we get here, we can't pre-commit anything more.
	}
	return res, false, nil
}

// loadProveCommitsMiner enqueue all pending prove-commits for the given miner. This is called on
// load to populate the commitQueue and should not need to be called later.
//
// It will drop any pre-commits that have already expired.
func (ss *simulationState) loadProveCommitsMiner(ctx context.Context, addr address.Address, minerState miner.State) error {
	// Find all pending prove commits and group by proof type. Really, there should never
	// (except during upgrades be more than one type.
	nextEpoch := ss.nextEpoch()
	nv := ss.sm.GetNtwkVersion(ctx, nextEpoch)
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

// filterProveCommits filters out expired prove-commits.
func (ss *simulationState) filterProveCommits(ctx context.Context, minerAddr address.Address, snos []abi.SectorNumber) (good, expired []abi.SectorNumber, err error) {
	_, minerState, err := ss.getMinerState(ctx, minerAddr)
	if err != nil {
		return nil, nil, err
	}

	nextEpoch := ss.nextEpoch()
	nv := ss.sm.GetNtwkVersion(ctx, nextEpoch)
	av := actors.VersionForNetwork(nv)

	good = make([]abi.SectorNumber, 0, len(snos))
	for _, sno := range snos {
		info, err := minerState.GetPrecommittedSector(sno)
		if err != nil {
			return nil, nil, err
		}
		if info == nil {
			expired = append(expired, sno)
			continue
		}
		msd := policy.GetMaxProveCommitDuration(av, info.Info.SealProof)
		if nextEpoch > info.PreCommitEpoch+msd {
			expired = append(expired, sno)
			continue
		}
		good = append(good, sno)
	}
	return good, expired, nil
}
