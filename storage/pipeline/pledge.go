package sealing

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
)

var initialPledgeNum = types.NewInt(110)
var initialPledgeDen = types.NewInt(100)

func (m *Sealing) pledgeForPower(ctx context.Context, addedPower abi.StoragePower) (abi.TokenAmount, error) {
	store := adt.WrapStore(ctx, cbor.NewCborStore(bstore.NewAPIBlockstore(m.Api)))

	// load power actor
	var (
		powerSmoothed    builtin.FilterEstimate
		pledgeCollateral abi.TokenAmount
	)
	if act, err := m.Api.StateGetActor(ctx, power.Address, types.EmptyTSK); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor: %w", err)
	} else if s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor state: %w", err)
	} else if p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total power: %w", err)
	} else if c, err := s.TotalLocked(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine pledge collateral: %w", err)
	} else {
		powerSmoothed = p
		pledgeCollateral = c
	}

	// load reward actor
	rewardActor, err := m.Api.StateGetActor(ctx, reward.Address, types.EmptyTSK)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor: %w", err)
	}

	rewardState, err := reward.Load(store, rewardActor)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor state: %w", err)
	}

	// get circulating supply
	circSupply, err := m.Api.StateVMCirculatingSupplyInternal(ctx, types.EmptyTSK)
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting circulating supply: %w", err)
	}

	// do the calculation
	initialPledge, err := rewardState.InitialPledgeForPower(
		addedPower,
		pledgeCollateral,
		&powerSmoothed,
		circSupply.FilCirculating,
	)
	if err != nil {
		return big.Zero(), xerrors.Errorf("calculating initial pledge: %w", err)
	}

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

func (m *Sealing) sectorWeight(ctx context.Context, sector SectorInfo, expiration abi.ChainEpoch) (abi.StoragePower, error) {
	spt, err := m.currentSealProof(ctx)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("getting seal proof type: %w", err)
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("getting sector size: %w", err)
	}

	ts, err := m.Api.ChainHead(ctx)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("getting chain head: %w", err)
	}

	// get verified deal infos
	var w, vw = big.Zero(), big.Zero()

	for _, piece := range sector.Pieces {
		if !piece.HasDealInfo() {
			// todo StateMinerInitialPledgeCollateral doesn't add cc/padding to non-verified weight, is that correct?
			continue
		}

		alloc, err := piece.GetAllocation(ctx, m.Api, ts.Key())
		if err != nil || alloc == nil {
			w = big.Add(w, abi.NewStoragePower(int64(piece.Piece().Size)))
			continue
		}

		vw = big.Add(vw, abi.NewStoragePower(int64(piece.Piece().Size)))
	}

	// load market actor
	duration := expiration - ts.Height()
	sectorWeight := builtin.QAPowerForWeight(ssize, duration, w, vw)

	return sectorWeight, nil
}
