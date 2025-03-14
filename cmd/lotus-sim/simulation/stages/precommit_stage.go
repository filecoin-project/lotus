package stages

import (
	"context"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"

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
	minPreCommitBatchSize = 1
	maxPreCommitBatchSize = 256
)

type PreCommitStage struct {
	funding   Funding
	committer Committer

	// The tiers represent the top 1%, top 10%, and everyone else. When sealing sectors, we seal
	// a group of sectors for the top 1%, a group (half that size) for the top 10%, and one
	// sector for everyone else. We determine these rates by looking at two power tables.
	// TODO Ideally we'd "learn" this distribution from the network. But this is good enough for
	// now.
	top1, top10, rest actorIter
	initialized       bool
}

func NewPreCommitStage(funding Funding, committer Committer) (*PreCommitStage, error) {
	return &PreCommitStage{
		funding:   funding,
		committer: committer,
	}, nil
}

func (*PreCommitStage) Name() string {
	return "pre-commit"
}

// PackMessages packs pre-commit messages until the block is full.
func (stage *PreCommitStage) PackMessages(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	if !stage.initialized {
		if err := stage.load(ctx, bb); err != nil {
			return err
		}
	}

	var (
		full                             bool
		top1Count, top10Count, restCount int
	)
	start := time.Now()
	defer func() {
		if _err != nil {
			return
		}
		bb.L().Debugw("packed pre commits",
			"done", top1Count+top10Count+restCount,
			"top1", top1Count,
			"top10", top10Count,
			"rest", restCount,
			"filled-block", full,
			"duration", time.Since(start),
		)
	}()

	var top1Miners, top10Miners, restMiners int
	for i := 0; ; i++ {
		var (
			minerAddr address.Address
			count     *int
		)

		// We pre-commit for the top 1%, 10%, and the of the network 1/3rd of the time each.
		// This won't yield the most accurate distribution... but it'll give us a good
		// enough distribution.
		switch {
		case (i%3) <= 0 && top1Miners < stage.top1.len():
			count = &top1Count
			minerAddr = stage.top1.next()
			top1Miners++
		case (i%3) <= 1 && top10Miners < stage.top10.len():
			count = &top10Count
			minerAddr = stage.top10.next()
			top10Miners++
		case (i%3) <= 2 && restMiners < stage.rest.len():
			count = &restCount
			minerAddr = stage.rest.next()
			restMiners++
		default:
			// Well, we've run through all miners.
			return nil
		}

		var (
			added int
			err   error
		)
		added, full, err = stage.packMiner(ctx, bb, minerAddr, maxProveCommitBatchSize)
		if err != nil {
			return xerrors.Errorf("failed to pack precommits for miner %s: %w", minerAddr, err)
		}
		*count += added
		if full {
			return nil
		}
	}
}

// packMiner packs count pre-commits for the given miner.
func (stage *PreCommitStage) packMiner(
	ctx context.Context, bb *blockbuilder.BlockBuilder,
	minerAddr address.Address, count int,
) (int, bool, error) {
	log := bb.L().With("miner", minerAddr)
	epoch := bb.Height()
	nv := bb.NetworkVersion()

	minerActor, err := bb.StateTree().GetActor(minerAddr)
	if err != nil {
		return 0, false, err
	}
	minerState, err := miner.Load(bb.ActorStore(), minerActor)
	if err != nil {
		return 0, false, err
	}

	minerInfo, err := minerState.Info()
	if err != nil {
		return 0, false, err
	}

	// Make sure the miner is funded.
	minerBalance, err := minerState.AvailableBalance(minerActor.Balance)
	if err != nil {
		return 0, false, err
	}

	if big.Cmp(minerBalance, MinimumFunds) < 0 {
		err := stage.funding.Fund(bb, minerAddr)
		if err != nil {
			if blockbuilder.IsOutOfGas(err) {
				return 0, true, nil
			}
			return 0, false, err
		}
	}

	// Generate pre-commits.
	sealType, err := miner.PreferredSealProofTypeFromWindowPoStType(
		nv, minerInfo.WindowPoStProofType, false,
	)
	if err != nil {
		return 0, false, err
	}

	sectorNos, err := minerState.UnallocatedSectorNumbers(count)
	if err != nil {
		return 0, false, err
	}

	maxExtension, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return 0, false, xerrors.Errorf("failed to get max extension: %w", err)
	}

	expiration := epoch + maxExtension
	infos := make([]minertypes.PreCommitSectorParams, len(sectorNos))
	for i, sno := range sectorNos {
		infos[i] = minertypes.PreCommitSectorParams{
			SealProof:     sealType,
			SectorNumber:  sno,
			SealedCID:     mock.MockCommR(minerAddr, sno),
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
			params := minertypes.PreCommitSectorBatchParams{
				Sectors: batch,
			}
			enc, err := actors.SerializeParams(&params)
			if err != nil {
				return added, false, err
			}
			// NOTE: just in-case, sendAndFund will "fund" and re-try for any message
			// that fails due to "insufficient funds".
			if _, err := stage.funding.SendAndFund(bb, &types.Message{
				To:     minerAddr,
				From:   minerInfo.Worker,
				Value:  abi.NewTokenAmount(0),
				Method: builtin.MethodsMiner.PreCommitSectorBatch,
				Params: enc,
			}); blockbuilder.IsOutOfGas(err) {
				// try again with a smaller batch.
				targetBatchSize /= 2
				continue
			} else if aerr, ok := err.(aerrors.ActorError); ok && !aerr.IsFatal() {
				// Log the error and move on. No reason to stop.
				log.Errorw("failed to pre-commit for unknown reasons",
					"error", aerr,
					"sectors", batch,
				)
				return added, false, nil
			} else if err != nil {
				return added, false, err
			}

			for _, info := range batch {
				if err := stage.committer.EnqueueProveCommit(minerAddr, epoch, toSectorPreCommitInfo(info)); err != nil {
					return added, false, err
				}
				added++
			}
			infos = infos[len(batch):]
		}
	}
	for _, info := range infos {
		enc, err := actors.SerializeParams(&info) //nolint
		if err != nil {
			return 0, false, err
		}
		if _, err := stage.funding.SendAndFund(bb, &types.Message{
			To:     minerAddr,
			From:   minerInfo.Worker,
			Value:  abi.NewTokenAmount(0),
			Method: builtin.MethodsMiner.PreCommitSector,
			Params: enc,
		}); blockbuilder.IsOutOfGas(err) {
			return added, true, nil
		} else if err != nil {
			return added, false, err
		}

		if err := stage.committer.EnqueueProveCommit(minerAddr, epoch, toSectorPreCommitInfo(info)); err != nil {
			return added, false, err
		}
		added++
	}
	return added, false, nil
}

func (stage *PreCommitStage) load(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	bb.L().Infow("loading miner power for pre-commits")
	start := time.Now()
	defer func() {
		if _err != nil {
			return
		}
		bb.L().Infow("loaded miner power for pre-commits",
			"duration", time.Since(start),
			"top1", stage.top1.len(),
			"top10", stage.top10.len(),
			"rest", stage.rest.len(),
		)
	}()

	store := bb.ActorStore()
	st := bb.ParentStateTree()
	powerState, err := loadPower(store, st)
	if err != nil {
		return xerrors.Errorf("failed to power actor: %w", err)
	}

	type onboardingInfo struct {
		addr        address.Address
		sectorCount uint64
	}
	var sealList []onboardingInfo
	err = powerState.ForEachClaim(func(addr address.Address, claim power.Claim) error {
		if claim.RawBytePower.IsZero() {
			return nil
		}

		minerState, err := loadMiner(store, st, addr)
		if err != nil {
			return err
		}
		info, err := minerState.Info()
		if err != nil {
			return err
		}

		sectorCount := sectorsFromClaim(info.SectorSize, claim)

		if sectorCount > 0 {
			sealList = append(sealList, onboardingInfo{addr, uint64(sectorCount)})
		}
		return nil
	}, false)
	if err != nil {
		return err
	}

	if len(sealList) == 0 {
		return xerrors.Errorf("simulation has no miners")
	}

	// Now that we have a list of sealing miners, sort them into percentiles.
	sort.Slice(sealList, func(i, j int) bool {
		return sealList[i].sectorCount < sealList[j].sectorCount
	})

	// reset, just in case.
	stage.top1 = actorIter{}
	stage.top10 = actorIter{}
	stage.rest = actorIter{}

	for i, oi := range sealList {
		var dist *actorIter
		if i < len(sealList)/100 {
			dist = &stage.top1
		} else if i < len(sealList)/10 {
			dist = &stage.top10
		} else {
			dist = &stage.rest
		}
		dist.add(oi.addr)
	}

	stage.top1.shuffle()
	stage.top10.shuffle()
	stage.rest.shuffle()

	stage.initialized = true
	return nil
}

func toSectorPreCommitInfo(param minertypes.PreCommitSectorParams) minertypes.SectorPreCommitInfo {
	return minertypes.SectorPreCommitInfo{
		SealProof:     param.SealProof,
		SectorNumber:  param.SectorNumber,
		SealedCID:     param.SealedCID,
		SealRandEpoch: param.SealRandEpoch,
		DealIDs:       param.DealIDs,
		Expiration:    param.Expiration,
		UnsealedCid:   nil,
	}
}
