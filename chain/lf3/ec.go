package lf3

import (
	"context"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type ecWrapper struct {
	ChainStore   *store.ChainStore
	StateManager *stmgr.StateManager
	Manifest     f3.Manifest
}

type tsWrapper struct {
	inner *types.TipSet
}

func (ts *tsWrapper) Key() gpbft.TipSetKey {
	return ts.inner.Key().Bytes()
}

func (ts *tsWrapper) Beacon() []byte {
	entries := ts.inner.Blocks()[0].BeaconEntries
	if len(entries) == 0 {
		return []byte{}
	}
	return entries[len(entries)-1].Data
}

func (ts *tsWrapper) Epoch() int64 {
	return int64(ts.inner.Height())
}

func (ts *tsWrapper) Timestamp() time.Time {
	return time.Unix(int64(ts.inner.Blocks()[0].Timestamp), 0)
}

func wrapTS(ts *types.TipSet) f3.TipSet {
	if ts == nil {
		return nil
	}
	return &tsWrapper{ts}
}

// GetTipsetByEpoch should return a tipset before the one requested if the requested
// tipset does not exist due to null epochs
func (ec *ecWrapper) GetTipsetByEpoch(ctx context.Context, epoch int64) (f3.TipSet, error) {
	ts, err := ec.ChainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(epoch), nil, true)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by height: %w", err)
	}
	return wrapTS(ts), nil
}

func (ec *ecWrapper) GetTipset(ctx context.Context, tsk gpbft.TipSetKey) (f3.TipSet, error) {
	tskLotus, err := types.TipSetKeyFromBytes(tsk)
	if err != nil {
		return nil, xerrors.Errorf("decoding tsk: %w", err)
	}

	ts, err := ec.ChainStore.GetTipSetFromKey(ctx, tskLotus)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by key: %w", err)
	}

	return wrapTS(ts), nil
}

func (ec *ecWrapper) GetHead(_ context.Context) (f3.TipSet, error) {
	return wrapTS(ec.ChainStore.GetHeaviestTipSet()), nil
}

func (ec *ecWrapper) GetParent(ctx context.Context, tsF3 f3.TipSet) (f3.TipSet, error) {
	var ts *types.TipSet
	if tsW, ok := tsF3.(*tsWrapper); ok {
		ts = tsW.inner
	} else {
		tskLotus, err := types.TipSetKeyFromBytes(tsF3.Key())
		if err != nil {
			return nil, xerrors.Errorf("decoding tsk: %w", err)
		}
		ts, err = ec.ChainStore.GetTipSetFromKey(ctx, tskLotus)
		if err != nil {
			return nil, xerrors.Errorf("getting tipset by key for get parent: %w", err)
		}
	}
	parentTs, err := ec.ChainStore.GetTipSetFromKey(ctx, ts.Parents())
	if err != nil {
		return nil, xerrors.Errorf("getting parent tipset: %w", err)
	}
	return wrapTS(parentTs), nil
}

func (ec *ecWrapper) GetPowerTable(ctx context.Context, tskF3 gpbft.TipSetKey) (gpbft.PowerEntries, error) {
	tskLotus, err := types.TipSetKeyFromBytes(tskF3)
	if err != nil {
		return nil, xerrors.Errorf("decoding tsk: %w", err)
	}
	ts, err := ec.ChainStore.GetTipSetFromKey(ctx, tskLotus)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by key for get parent: %w", err)
	}
	//log.Infof("collecting power table for: %d", ts.Height())
	stCid := ts.ParentState()

	sm := ec.StateManager

	powerAct, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return nil, xerrors.Errorf("loading power actor: %w", err)
	}

	powerState, err := power.Load(ec.ChainStore.ActorStore(ctx), powerAct)
	if err != nil {
		return nil, xerrors.Errorf("loading power actor state: %w", err)
	}

	var powerEntries gpbft.PowerEntries
	err = powerState.ForEachClaim(func(miner address.Address, claim power.Claim) error {
		if claim.QualityAdjPower.LessThanEqual(big.Zero()) {
			return nil
		}

		// TODO: optimize
		ok, err := powerState.MinerNominalPowerMeetsConsensusMinimum(miner)
		if err != nil {
			return xerrors.Errorf("checking consensus minimums: %w", err)
		}
		if !ok {
			return nil
		}

		id, err := address.IDFromAddress(miner)
		if err != nil {
			return xerrors.Errorf("transforming address to ID: %w", err)
		}

		pe := gpbft.PowerEntry{
			ID:    gpbft.ActorID(id),
			Power: claim.QualityAdjPower.Int,
		}
		waddr, err := stmgr.GetMinerWorkerRaw(ctx, sm, stCid, miner)
		if err != nil {
			return xerrors.Errorf("GetMinerWorkerRaw failed: %w", err)
		}
		if waddr.Protocol() != address.BLS {
			return xerrors.Errorf("wrong type of worker address")
		}
		pe.PubKey = gpbft.PubKey(waddr.Payload())
		powerEntries = append(powerEntries, pe)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("collecting the power table: %w", err)
	}

	sort.Sort(powerEntries)
	return powerEntries, nil
}
