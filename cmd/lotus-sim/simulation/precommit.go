package simulation

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	tutils "github.com/filecoin-project/specs-actors/v5/support/testing"
)

var (
	targetFunds = abi.TokenAmount(types.MustParseFIL("1000FIL"))
	minFunds    = abi.TokenAmount(types.MustParseFIL("100FIL"))
)

// makeCommR generates a "fake" but valid CommR for a sector. It is unique for the given sector/miner.
func makeCommR(minerAddr address.Address, sno abi.SectorNumber) cid.Cid {
	return tutils.MakeCID(fmt.Sprintf("%s:%d", minerAddr, sno), &miner5.SealedCIDPrefix)
}

// packPreCommits packs pre-commit messages until the block is full.
func (ss *simulationState) packPreCommits(ctx context.Context, cb packFunc) (_err error) {
	var (
		full                             bool
		top1Count, top10Count, restCount int
	)
	defer func() {
		if _err != nil {
			return
		}
		log.Debugw("packed pre commits",
			"done", top1Count+top10Count+restCount,
			"top1", top1Count,
			"top10", top10Count,
			"rest", restCount,
			"filled-block", full,
		)
	}()

	var top1Miners, top10Miners, restMiners int
	for i := 0; ; i++ {
		var (
			minerAddr address.Address
			count     *int
		)

		// We pre-commit for the top 1%, 10%, and the of the network 1/3rd of the time each.
		// This won't yeild the most accurate distribution... but it'll give us a good
		// enough distribution.

		// NOTE: We submit at most _one_ 819 sector batch per-miner per-block. See the
		// comment on packPreCommitsMiner for why. We should fix this.
		switch {
		case (i%3) <= 0 && top1Miners < ss.minerDist.top1.len():
			count = &top1Count
			minerAddr = ss.minerDist.top1.next()
			top1Miners++
		case (i%3) <= 1 && top10Miners < ss.minerDist.top10.len():
			count = &top10Count
			minerAddr = ss.minerDist.top10.next()
			top10Miners++
		case (i%3) <= 2 && restMiners < ss.minerDist.rest.len():
			count = &restCount
			minerAddr = ss.minerDist.rest.next()
			restMiners++
		default:
			// Well, we've run through all miners.
			return nil
		}

		var (
			added int
			err   error
		)
		added, full, err = ss.packPreCommitsMiner(ctx, cb, minerAddr, maxProveCommitBatchSize)
		if err != nil {
			return xerrors.Errorf("failed to pack precommits for miner %s: %w", minerAddr, err)
		}
		*count += added
		if full {
			return nil
		}
	}
}

// packPreCommitsMiner packs count pre-commits for the given miner. This should only be called once
// per-miner, per-epoch to avoid packing multiple pre-commits with the same sector numbers.
func (ss *simulationState) packPreCommitsMiner(ctx context.Context, cb packFunc, minerAddr address.Address, count int) (int, bool, error) {
	// Load everything.
	epoch := ss.nextEpoch()
	nv := ss.StateManager.GetNtwkVersion(ctx, epoch)
	actor, minerState, err := ss.getMinerState(ctx, minerAddr)
	if err != nil {
		return 0, false, err
	}

	minerInfo, err := ss.getMinerInfo(ctx, minerAddr)
	if err != nil {
		return 0, false, err
	}

	// Make sure the miner is funded.
	minerBalance, err := minerState.AvailableBalance(actor.Balance)
	if err != nil {
		return 0, false, err
	}

	if big.Cmp(minerBalance, minFunds) < 0 {
		if _, err := cb(&types.Message{
			From:   builtin.BurntFundsActorAddr,
			To:     minerAddr,
			Value:  targetFunds,
			Method: builtin.MethodSend,
		}); err == ErrOutOfGas {
			return 0, true, nil
		} else if err != nil {
			return 0, false, xerrors.Errorf("failed to fund miner %s: %w", minerAddr, err)
		}
	}

	// Generate pre-commits.
	sealType, err := miner.PreferredSealProofTypeFromWindowPoStType(
		nv, minerInfo.WindowPoStProofType,
	)
	if err != nil {
		return 0, false, err
	}

	sectorNos, err := minerState.UnallocatedSectorNumbers(count)
	if err != nil {
		return 0, false, err
	}

	expiration := epoch + policy.GetMaxSectorExpirationExtension()
	infos := make([]miner.SectorPreCommitInfo, len(sectorNos))
	for i, sno := range sectorNos {
		infos[i] = miner.SectorPreCommitInfo{
			SealProof:     sealType,
			SectorNumber:  sno,
			SealedCID:     makeCommR(minerAddr, sno),
			SealRandEpoch: epoch - 1,
			Expiration:    expiration,
		}
	}

	// Commit the pre-commits.
	added := 0
	if nv >= network.Version13 {
		targetBatchSize := maxPreCommitBatchSize
		for targetBatchSize >= minPreCommitBatchSize && len(infos) >= minPreCommitBatchSize {
			batch := infos
			if len(batch) > targetBatchSize {
				batch = batch[:targetBatchSize]
			}
			params := miner5.PreCommitSectorBatchParams{
				Sectors: batch,
			}
			enc, err := actors.SerializeParams(&params)
			if err != nil {
				return 0, false, err
			}
			// NOTE: just in-case, sendAndFund will "fund" and re-try for any message
			// that fails due to "insufficient funds".
			if _, err := sendAndFund(cb, &types.Message{
				To:     minerAddr,
				From:   minerInfo.Worker,
				Value:  abi.NewTokenAmount(0),
				Method: miner.Methods.PreCommitSectorBatch,
				Params: enc,
			}); err == ErrOutOfGas {
				// try again with a smaller batch.
				targetBatchSize /= 2
				continue
			} else if err != nil {
				return 0, false, err
			}

			for _, info := range batch {
				if err := ss.commitQueue.enqueueProveCommit(minerAddr, epoch, info); err != nil {
					return 0, false, err
				}
				added++
			}
			infos = infos[len(batch):]
		}
	}
	for _, info := range infos {
		enc, err := actors.SerializeParams(&info)
		if err != nil {
			return 0, false, err
		}
		if _, err := sendAndFund(cb, &types.Message{
			To:     minerAddr,
			From:   minerInfo.Worker,
			Value:  abi.NewTokenAmount(0),
			Method: miner.Methods.PreCommitSector,
			Params: enc,
		}); err == ErrOutOfGas {
			return added, true, nil
		} else if err != nil {
			return added, false, err
		}

		if err := ss.commitQueue.enqueueProveCommit(minerAddr, epoch, info); err != nil {
			return 0, false, err
		}
		added++
	}
	return added, false, nil
}
