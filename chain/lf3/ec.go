package lf3

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/ec"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"

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
	chainStore   *store.ChainStore
	syncer       *chain.Syncer
	stateManager *stmgr.StateManager
	cache        *lru.TwoQueueCache[types.TipSetKey, gpbft.PowerEntries]

	powerTableComputeLock sync.Mutex
	powerTableComputeJobs map[types.TipSetKey]chan struct{}
	powerTableComputeSema chan struct{}

	mapReduceCache builtin.MapReduceCache
}

func newEcWrapper(chainStore *store.ChainStore, syncer *chain.Syncer, stateManager *stmgr.StateManager) *ecWrapper {
	cache, err := lru.New2Q[types.TipSetKey, gpbft.PowerEntries](128)
	if err != nil {
		panic(err)
	}
	return &ecWrapper{
		chainStore:   chainStore,
		syncer:       syncer,
		stateManager: stateManager,
		cache:        cache,

		powerTableComputeJobs: make(map[types.TipSetKey]chan struct{}),
		powerTableComputeSema: make(chan struct{}, min(4, runtime.NumCPU()/2)),
	}
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
	ts, err := ec.chainStore.GetTipsetByHeight(ctx, abi.ChainEpoch(epoch), nil, true)
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
	head := ec.chainStore.GetHeaviestTipSet()
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
	parentTs, err := ec.chainStore.GetTipSetFromKey(ctx, ts.Parents())
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
	// Either wait for someone else to compute the power table, or claim the job.
	for {
		// check the cache
		pe, ok := ec.cache.Get(tsk)
		if ok {
			return pe, nil
		}
		ec.powerTableComputeLock.Lock()
		waitCh, ok := ec.powerTableComputeJobs[tsk]
		if !ok {
			break
		}
		ec.powerTableComputeLock.Unlock()
		select {
		case <-waitCh:
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Ok, we have the lock and nobody else has claimed the job. Claim it.
	myWaitCh := make(chan struct{})
	ec.powerTableComputeJobs[tsk] = myWaitCh
	ec.powerTableComputeLock.Unlock()

	// Make sure to "unlock" the job when we're done, even if we don't complete it. Someone else
	// will complete it in that case.
	defer func() {
		ec.powerTableComputeLock.Lock()
		delete(ec.powerTableComputeJobs, tsk)
		ec.powerTableComputeLock.Unlock()

		close(myWaitCh)
	}()

	// Then wait in line. We only allow 4 jobs at once.
	select {
	case ec.powerTableComputeSema <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() {
		<-ec.powerTableComputeSema
	}()

	// Finally, do the actual compute.
	ts, err := ec.chainStore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset by key for get parent: %w", err)
	}

	state, err := ec.stateManager.ParentState(ts)
	if err != nil {
		return nil, xerrors.Errorf("loading the state tree: %w", err)
	}
	powerAct, err := state.GetActor(power.Address)
	if err != nil {
		return nil, xerrors.Errorf("getting the power actor: %w", err)
	}

	powerState, err := power.Load(ec.chainStore.ActorStore(ctx), powerAct)
	if err != nil {
		return nil, xerrors.Errorf("loading power actor state: %w", err)
	}

	claims, err := powerState.CollectEligibleClaims(&ec.mapReduceCache)
	if err != nil {
		return nil, xerrors.Errorf("collecting valid claims: %w", err)
	}
	var powerEntries gpbft.PowerEntries
	for _, claim := range claims {
		if claim.QualityAdjPower.Sign() <= 0 {
			continue
		}

		id, err := address.IDFromAddress(claim.Address)
		if err != nil {
			return nil, xerrors.Errorf("transforming address to ID: %w", err)
		}

		pe := gpbft.PowerEntry{
			ID:    gpbft.ActorID(id),
			Power: claim.QualityAdjPower,
		}

		act, err := state.GetActor(claim.Address)
		if err != nil {
			return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
		}
		mstate, err := miner.Load(ec.chainStore.ActorStore(ctx), act)
		if err != nil {
			return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
		}

		info, err := mstate.Info()
		if err != nil {
			return nil, xerrors.Errorf("failed to load actor info: %w", err)
		}
		// check fee debt
		if debt, err := mstate.FeeDebt(); err != nil {
			return nil, err
		} else if !debt.IsZero() {
			// fee debt don't add the miner to power table
			continue
		}
		// check consensus faults
		if ts.Height() <= info.ConsensusFaultElapsed {
			continue
		}

		waddr, err := vm.ResolveToDeterministicAddr(state, ec.chainStore.ActorStore(ctx), info.Worker)
		if err != nil {
			return nil, xerrors.Errorf("resolve miner worker address: %w", err)
		}

		if waddr.Protocol() != address.BLS {
			return nil, xerrors.Errorf("wrong type of worker address")
		}
		pe.PubKey = waddr.Payload()
		powerEntries = append(powerEntries, pe)
	}

	sort.Sort(powerEntries)
	ec.cache.Add(tsk, powerEntries)

	return powerEntries, nil
}

func (ec *ecWrapper) Finalize(ctx context.Context, key gpbft.TipSetKey) error {
	tsk, err := toLotusTipSetKey(key)
	if err != nil {
		return err
	}
	if err = ec.syncer.SyncCheckpoint(ctx, tsk); err != nil {
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
	ts, err := ec.chainStore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset from key: %w", err)
	}
	return ts, nil
}

func toLotusTipSetKey(key gpbft.TipSetKey) (types.TipSetKey, error) {
	return types.TipSetKeyFromBytes(key)
}
