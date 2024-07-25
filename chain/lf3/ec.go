package lf3

import (
	"context"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/ec"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

type ecWrapper struct {
	ChainStore   *store.ChainStore
	StateManager *stmgr.StateManager
}

var _ ec.TipSet = (*f3TipSet)(nil)

type f3TipSet types.TipSet

func (ts *f3TipSet) cast() *types.TipSet {
	return (*types.TipSet)(ts)
}

func (ts *f3TipSet) String() string {
	return ts.cast().String()
}

func (ts *f3TipSet) Key() gpbft.TipSetKey {
	return ts.cast().Key().Bytes()
}

func (ts *f3TipSet) Beacon() []byte {
	entries := ts.cast().Blocks()[0].BeaconEntries
	if len(entries) == 0 {
		// This should never happen in practice, but set beacon to a non-nil
		// 32byte slice to force the message builder to generate a
		// ticket. Otherwise, messages that require ticket, i.e. CONVERGE will fail
		// validation due to the absence of ticket. This is a convoluted way of doing it.
		return make([]byte, 32)
	}
	return entries[len(entries)-1].Data
}

func (ts *f3TipSet) Epoch() int64 {
	return int64(ts.cast().Height())
}

func (ts *f3TipSet) Timestamp() time.Time {
	return time.Unix(int64(ts.cast().Blocks()[0].Timestamp), 0)
}

func wrapTS(ts *types.TipSet) ec.TipSet {
	if ts == nil {
		return nil
	}
	return (*f3TipSet)(ts)
}

// GetTipsetByEpoch should return a tipset before the one requested if the requested
// tipset does not exist due to null epochs
func (ec *ecWrapper) GetTipsetByEpoch(ctx context.Context, epoch int64) (ec.TipSet, error) {
	ts, err := ec.ChainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(epoch), nil, true)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by height: %w", err)
	}
	return wrapTS(ts), nil
}

func (ec *ecWrapper) GetTipset(ctx context.Context, tsk gpbft.TipSetKey) (ec.TipSet, error) {
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

func (ec *ecWrapper) GetHead(_ context.Context) (ec.TipSet, error) {
	return wrapTS(ec.ChainStore.GetHeaviestTipSet()), nil
}

func (ec *ecWrapper) GetParent(ctx context.Context, tsF3 ec.TipSet) (ec.TipSet, error) {
	var ts *types.TipSet
	if tsW, ok := tsF3.(*f3TipSet); ok {
		ts = tsW.cast()
	} else {
		// There are only two implementations of ec.TipSet: f3TipSet, and one in fake EC
		// backend.
		//
		// TODO: Revisit the type check here and remove as needed once testing
		//       is over and fake EC backend goes away.
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
	tsk, err := types.TipSetKeyFromBytes(tskF3)
	if err != nil {
		return nil, xerrors.Errorf("decoding tsk: %w", err)
	}
	return ec.getPowerTableLotusTSK(ctx, tsk)
}

func (ec *ecWrapper) getPowerTableLotusTSK(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	ts, err := ec.ChainStore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by key for get parent: %w", err)
	}

	state, err := ec.StateManager.ParentState(ts)
	if err != nil {
		return nil, xerrors.Errorf("loading the state tree: %w", err)
	}
	powerAct, err := state.GetActor(power.Address)
	if err != nil {
		return nil, xerrors.Errorf("getting the power actor: %w", err)
	}

	powerState, err := power.Load(ec.ChainStore.ActorStore(ctx), powerAct)
	if err != nil {
		return nil, xerrors.Errorf("loading power actor state: %w", err)
	}

	var powerEntries gpbft.PowerEntries
	err = powerState.ForEachClaim(func(minerAddr address.Address, claim power.Claim) error {
		if claim.QualityAdjPower.Sign() <= 0 {
			return nil
		}

		// TODO: optimize
		ok, err := powerState.MinerNominalPowerMeetsConsensusMinimum(minerAddr)
		if err != nil {
			return xerrors.Errorf("checking consensus minimums: %w", err)
		}
		if !ok {
			return nil
		}

		id, err := address.IDFromAddress(minerAddr)
		if err != nil {
			return xerrors.Errorf("transforming address to ID: %w", err)
		}

		pe := gpbft.PowerEntry{
			ID:    gpbft.ActorID(id),
			Power: claim.QualityAdjPower,
		}

		act, err := state.GetActor(minerAddr)
		if err != nil {
			return xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
		}
		mstate, err := miner.Load(ec.ChainStore.ActorStore(ctx), act)
		if err != nil {
			return xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
		}

		info, err := mstate.Info()
		if err != nil {
			return xerrors.Errorf("failed to load actor info: %w", err)
		}
		// check fee debt
		if debt, err := mstate.FeeDebt(); err != nil {
			return err
		} else if !debt.IsZero() {
			// fee debt don't add the miner to power table
			return nil
		}
		// check consensus faults
		if ts.Height() <= info.ConsensusFaultElapsed {
			return nil
		}

		waddr, err := vm.ResolveToDeterministicAddr(state, ec.ChainStore.ActorStore(ctx), info.Worker)
		if err != nil {
			return xerrors.Errorf("resolve miner worker address: %w", err)
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
