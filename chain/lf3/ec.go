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

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

var (
	_ ec.Backend = (*ecWrapper)(nil)
	_ ec.TipSet  = (*f3TipSet)(nil)
)

type ecWrapper struct {
	ChainStore   *store.ChainStore
	Syncer       *chain.Syncer
	StateManager *stmgr.StateManager
}

type f3TipSet struct {
	*types.TipSet
}

func (ts *f3TipSet) String() string       { return ts.TipSet.String() }
func (ts *f3TipSet) Key() gpbft.TipSetKey { return ts.TipSet.Key().Bytes() }
func (ts *f3TipSet) Epoch() int64         { return int64(ts.TipSet.Height()) }

func (ts *f3TipSet) FirstBlockHeader() *types.BlockHeader {
	if ts.TipSet == nil || len(ts.TipSet.Blocks()) == 0 {
		return nil
	}
	return ts.TipSet.Blocks()[0]
}

func (ts *f3TipSet) Beacon() []byte {
	switch header := ts.FirstBlockHeader(); {
	case header == nil, len(header.BeaconEntries) == 0:
		// This should never happen in practice, but set beacon to a non-nil 32byte slice
		// to force the message builder to generate a ticket. Otherwise, messages that
		// require ticket, i.e. CONVERGE will fail validation due to the absence of
		// ticket. This is a convoluted way of doing it.

		// TODO: investigate if this is still necessary, or how message builder can be
		//       adapted to behave correctly regardless of beacon value, e.g. fail fast
		//       instead of building CONVERGE with empty beacon.
		return make([]byte, 32)
	default:
		return header.BeaconEntries[len(header.BeaconEntries)-1].Data
	}
}

func (ts *f3TipSet) Timestamp() time.Time {
	if header := ts.FirstBlockHeader(); header != nil {
		return time.Unix(int64(header.Timestamp), 0)
	}
	return time.Time{}
}

// GetTipsetByEpoch should return a tipset before the one requested if the requested
// tipset does not exist due to null epochs
func (ec *ecWrapper) GetTipsetByEpoch(ctx context.Context, epoch int64) (ec.TipSet, error) {
	ts, err := ec.ChainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(epoch), nil, true)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by height: %w", err)
	}
	return &f3TipSet{TipSet: ts}, nil
}

func (ec *ecWrapper) GetTipset(ctx context.Context, tsk gpbft.TipSetKey) (ec.TipSet, error) {
	ts, err := ec.getTipSetFromF3TSK(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by key: %w", err)
	}

	return &f3TipSet{TipSet: ts}, nil
}

func (ec *ecWrapper) GetHead(context.Context) (ec.TipSet, error) {
	head := ec.ChainStore.GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("no heaviest tipset")
	}
	return &f3TipSet{TipSet: head}, nil
}

func (ec *ecWrapper) GetParent(ctx context.Context, tsF3 ec.TipSet) (ec.TipSet, error) {
	ts, err := ec.toLotusTipSet(ctx, tsF3)
	if err != nil {
		return nil, err
	}
	parentTs, err := ec.ChainStore.GetTipSetFromKey(ctx, ts.Parents())
	if err != nil {
		return nil, xerrors.Errorf("getting parent tipset: %w", err)
	}
	return &f3TipSet{TipSet: parentTs}, nil
}

func (ec *ecWrapper) GetPowerTable(ctx context.Context, tskF3 gpbft.TipSetKey) (gpbft.PowerEntries, error) {
	tsk, err := toLotusTipSetKey(tskF3)
	if err != nil {
		return nil, err
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
		pe.PubKey = waddr.Payload()
		powerEntries = append(powerEntries, pe)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("collecting the power table: %w", err)
	}

	sort.Sort(powerEntries)
	return powerEntries, nil
}

func (ec *ecWrapper) Finalize(ctx context.Context, key gpbft.TipSetKey) error {
	tsk, err := toLotusTipSetKey(key)
	if err != nil {
		return err
	}
	if err = ec.Syncer.SyncCheckpoint(ctx, tsk); err != nil {
		return xerrors.Errorf("checkpointing finalized tipset: %w", err)
	}
	return nil
}

func (ec *ecWrapper) toLotusTipSet(ctx context.Context, ts ec.TipSet) (*types.TipSet, error) {
	switch tst := ts.(type) {
	case *f3TipSet:
		return tst.TipSet, nil
	default:
		// Fall back on getting the tipset by key. This path is executed only in testing.
		return ec.getTipSetFromF3TSK(ctx, ts.Key())
	}
}

func (ec *ecWrapper) getTipSetFromF3TSK(ctx context.Context, key gpbft.TipSetKey) (*types.TipSet, error) {
	tsk, err := toLotusTipSetKey(key)
	if err != nil {
		return nil, err
	}
	ts, err := ec.ChainStore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset from key: %w", err)
	}
	return ts, nil
}

func toLotusTipSetKey(key gpbft.TipSetKey) (types.TipSetKey, error) {
	return types.TipSetKeyFromBytes(key)
}
