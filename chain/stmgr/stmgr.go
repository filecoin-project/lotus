package stmgr

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v8/actors/migration/nv16"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"

	// Used for genesis.
	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
)

const LookbackNoLimit = api.LookbackNoLimit
const ReceiptAmtBitwidth = 3

var execTraceCacheSize = 16
var log = logging.Logger("statemgr")

type StateManagerAPI interface {
	Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)
	GetPaychState(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, paych.State, error)
	LoadActorTsk(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error)
	LookupIDAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)
	ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)
}

type versionSpec struct {
	networkVersion network.Version
	atOrBelow      abi.ChainEpoch
}

type migration struct {
	upgrade              MigrationFunc
	preMigrations        []PreMigration
	cache                *nv16.MemMigrationCache
	migrationResultCache *migrationResultCache
}

type migrationResultCache struct {
	ds        dstore.Batching
	keyPrefix string
}

func (m *migrationResultCache) keyForMigration(root cid.Cid) dstore.Key {
	kStr := fmt.Sprintf("%s/%s", m.keyPrefix, root)
	return dstore.NewKey(kStr)
}

func init() {
	if s := os.Getenv("LOTUS_EXEC_TRACE_CACHE_SIZE"); s != "" {
		letc, err := strconv.Atoi(s)
		if err != nil {
			log.Errorf("failed to parse 'LOTUS_EXEC_TRACE_CACHE_SIZE' env var: %s", err)
		} else {
			execTraceCacheSize = letc
		}
	}
}

func (m *migrationResultCache) Get(ctx context.Context, root cid.Cid) (cid.Cid, bool, error) {
	k := m.keyForMigration(root)

	bs, err := m.ds.Get(ctx, k)
	if ipld.IsNotFound(err) {
		return cid.Undef, false, nil
	} else if err != nil {
		return cid.Undef, false, xerrors.Errorf("error loading migration result: %w", err)
	}

	c, err := cid.Parse(bs)
	if err != nil {
		return cid.Undef, false, xerrors.Errorf("error parsing migration result: %w", err)
	}

	return c, true, nil
}

func (m *migrationResultCache) Store(ctx context.Context, root cid.Cid, resultCid cid.Cid) error {
	k := m.keyForMigration(root)
	if err := m.ds.Put(ctx, k, resultCid.Bytes()); err != nil {
		return err
	}

	return nil
}

func (m *migrationResultCache) Delete(ctx context.Context, root cid.Cid) {
	_ = m.ds.Delete(ctx, m.keyForMigration(root))
}

type Executor interface {
	NewActorRegistry() *vm.ActorRegistry
	ExecuteTipSet(ctx context.Context, sm *StateManager, ts *types.TipSet, em ExecMonitor, vmTracing bool) (stateroot cid.Cid, rectsroot cid.Cid, err error)
}

type StateManager struct {
	cs *store.ChainStore

	cancel   context.CancelFunc
	shutdown chan struct{}

	// Determines the network version at any given epoch.
	networkVersions []versionSpec
	latestVersion   network.Version

	// Maps chain epochs to migrations.
	stateMigrations map[abi.ChainEpoch]*migration
	// A set of potentially expensive/time consuming upgrades. Explicit
	// calls for, e.g., gas estimation fail against this epoch with
	// ErrExpensiveFork.
	expensiveUpgrades map[abi.ChainEpoch]struct{}

	stCache             map[string][]cid.Cid
	tCache              treeCache
	compWait            map[string]chan struct{}
	stlk                sync.Mutex
	genesisMsigLk       sync.Mutex
	newVM               func(context.Context, *vm.VMOpts) (vm.Interface, error)
	Syscalls            vm.SyscallBuilder
	preIgnitionVesting  []msig0.State
	postIgnitionVesting []msig0.State
	postCalicoVesting   []msig0.State

	genesisPledge abi.TokenAmount

	tsExec        Executor
	tsExecMonitor ExecMonitor
	beacon        beacon.Schedule

	chainIndexer index.Indexer

	// We keep a small cache for calls to ExecutionTrace which helps improve
	// performance for node operators like exchanges and block explorers
	execTraceCache *arc.ARCCache[types.TipSetKey, tipSetCacheEntry]
	// We need a lock while making the copy as to prevent other callers
	// overwrite the cache while making the copy
	execTraceCacheLock sync.Mutex
}

// Caches a single state tree
type treeCache struct {
	root cid.Cid
	tree *state.StateTree
}

type tipSetCacheEntry struct {
	postStateRoot cid.Cid
	invocTrace    []*api.InvocResult
}

func NewStateManager(cs *store.ChainStore, exec Executor, sys vm.SyscallBuilder, us UpgradeSchedule, beacon beacon.Schedule,
	metadataDs dstore.Batching, chainIndexer index.Indexer) (*StateManager, error) {
	// If we have upgrades, make sure they're in-order and make sense.
	if err := us.Validate(); err != nil {
		return nil, err
	}

	stateMigrations := make(map[abi.ChainEpoch]*migration, len(us))
	expensiveUpgrades := make(map[abi.ChainEpoch]struct{}, len(us))
	var networkVersions []versionSpec
	lastVersion := buildconstants.GenesisNetworkVersion
	if len(us) > 0 {
		// If we have any upgrades, process them and create a version
		// schedule.
		for _, upgrade := range us {
			if upgrade.Migration != nil || upgrade.PreMigrations != nil {
				migration := &migration{
					upgrade:       upgrade.Migration,
					preMigrations: upgrade.PreMigrations,
					cache:         nv16.NewMemMigrationCache(),
					migrationResultCache: &migrationResultCache{
						keyPrefix: fmt.Sprintf("/migration-cache/nv%d", upgrade.Network),
						ds:        metadataDs,
					},
				}

				stateMigrations[upgrade.Height] = migration
			}
			if upgrade.Expensive {
				expensiveUpgrades[upgrade.Height] = struct{}{}
			}

			networkVersions = append(networkVersions, versionSpec{
				networkVersion: lastVersion,
				atOrBelow:      upgrade.Height,
			})
			lastVersion = upgrade.Network
		}
	}

	log.Debugf("execTraceCache size: %d", execTraceCacheSize)
	var execTraceCache *arc.ARCCache[types.TipSetKey, tipSetCacheEntry]
	var err error
	if execTraceCacheSize > 0 {
		execTraceCache, err = arc.NewARC[types.TipSetKey, tipSetCacheEntry](execTraceCacheSize)
		if err != nil {
			return nil, err
		}
	}

	return &StateManager{
		networkVersions:   networkVersions,
		latestVersion:     lastVersion,
		stateMigrations:   stateMigrations,
		expensiveUpgrades: expensiveUpgrades,
		newVM:             vm.NewVM,
		Syscalls:          sys,
		cs:                cs,
		tsExec:            exec,
		stCache:           make(map[string][]cid.Cid),
		beacon:            beacon,
		tCache: treeCache{
			root: cid.Undef,
			tree: nil,
		},
		compWait:       make(map[string]chan struct{}),
		chainIndexer:   chainIndexer,
		execTraceCache: execTraceCache,
	}, nil
}

func NewStateManagerWithUpgradeScheduleAndMonitor(cs *store.ChainStore, exec Executor, sys vm.SyscallBuilder, us UpgradeSchedule, b beacon.Schedule, em ExecMonitor, metadataDs dstore.Batching, chainIndexer index.Indexer) (*StateManager, error) {
	sm, err := NewStateManager(cs, exec, sys, us, b, metadataDs, chainIndexer)
	if err != nil {
		return nil, err
	}
	sm.tsExecMonitor = em
	return sm, nil
}

func cidsToKey(cids []cid.Cid) string {
	var out string
	for _, c := range cids {
		out += c.KeyString()
	}
	return out
}

// Start starts the state manager's optional background processes. At the moment, this schedules
// pre-migration functions to run ahead of network upgrades.
//
// This method is not safe to invoke from multiple threads or concurrently with Stop.
func (sm *StateManager) Start(context.Context) error {
	var ctx context.Context
	ctx, sm.cancel = context.WithCancel(context.Background())
	sm.shutdown = make(chan struct{})
	go sm.preMigrationWorker(ctx)
	return nil
}

// Stop starts the state manager's background processes.
//
// This method is not safe to invoke concurrently with Start.
func (sm *StateManager) Stop(ctx context.Context) error {
	if sm.cancel != nil {
		sm.cancel()
		select {
		case <-sm.shutdown:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (sm *StateManager) ChainStore() *store.ChainStore {
	return sm.cs
}

func (sm *StateManager) Beacon() beacon.Schedule {
	return sm.beacon
}

// ResolveToDeterministicAddress is similar to `vm.ResolveToDeterministicAddr` but does not allow `Actor` type of addresses.
// Uses the `TipSet` `ts` to generate the VM state.
func (sm *StateManager) ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	switch addr.Protocol() {
	case address.BLS, address.SECP256K1, address.Delegated:
		return addr, nil
	case address.Actor:
		return address.Undef, xerrors.New("cannot resolve actor address to key address")
	default:
	}

	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	cst := cbor.NewCborStore(sm.cs.StateBlockstore())

	// First try to resolve the actor in the parent state, so we don't have to compute anything.
	tree, err := state.LoadStateTree(cst, ts.ParentState())
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to load parent state tree at tipset %s: %w", ts.Parents(), err)
	}

	resolved, err := vm.ResolveToDeterministicAddr(tree, cst, addr)
	if err == nil {
		return resolved, nil
	}

	// If that fails, compute the tip-set and try again.
	st, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("resolve address failed to get tipset %s state: %w", ts, err)
	}

	tree, err = state.LoadStateTree(cst, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to load state tree at tipset %s: %w", ts, err)
	}

	return vm.ResolveToDeterministicAddr(tree, cst, addr)
}

// ResolveToDeterministicAddressAtFinality is similar to stmgr.ResolveToDeterministicAddress but fails if the ID address being resolved isn't reorg-stable yet.
// It should not be used for consensus-critical subsystems.
func (sm *StateManager) ResolveToDeterministicAddressAtFinality(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	switch addr.Protocol() {
	case address.BLS, address.SECP256K1, address.Delegated:
		return addr, nil
	case address.Actor:
		return address.Undef, xerrors.New("cannot resolve actor address to key address")
	default:
	}

	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	var err error
	if ts.Height() > policy.ChainFinality {
		ts, err = sm.ChainStore().GetTipsetByHeight(ctx, ts.Height()-policy.ChainFinality, ts, true)
		if err != nil {
			return address.Undef, xerrors.Errorf("failed to load lookback tipset: %w", err)
		}
	}

	cst := cbor.NewCborStore(sm.cs.StateBlockstore())
	tree := sm.tCache.tree

	if tree == nil || sm.tCache.root != ts.ParentState() {
		tree, err = state.LoadStateTree(cst, ts.ParentState())
		if err != nil {
			return address.Undef, xerrors.Errorf("failed to load parent state tree: %w", err)
		}

		sm.tCache = treeCache{
			root: ts.ParentState(),
			tree: tree,
		}
	}

	resolved, err := vm.ResolveToDeterministicAddr(tree, cst, addr)
	if err == nil {
		return resolved, nil
	}

	return address.Undef, xerrors.New("ID address not found in lookback state")
}

func (sm *StateManager) GetBlsPublicKey(ctx context.Context, addr address.Address, ts *types.TipSet) (pubk []byte, err error) {
	kaddr, err := sm.ResolveToDeterministicAddress(ctx, addr, ts)
	if err != nil {
		return pubk, xerrors.Errorf("failed to resolve address to key address: %w", err)
	}

	if kaddr.Protocol() != address.BLS {
		return pubk, xerrors.Errorf("address must be BLS address to load bls public key")
	}

	return kaddr.Payload(), nil
}

func (sm *StateManager) LookupIDAddress(_ context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	// Check for the fast route first to avoid unnecessary CBOR store instantiation and state tree load.
	if addr.Protocol() == address.ID {
		return addr, nil
	}

	cst := cbor.NewCborStore(sm.cs.StateBlockstore())
	state, err := state.LoadStateTree(cst, sm.parentState(ts))
	if err != nil {
		return address.Undef, xerrors.Errorf("load state tree: %w", err)
	}
	return state.LookupIDAddress(addr)
}

func (sm *StateManager) LookupID(ctx context.Context, addr address.Address, ts *types.TipSet) (abi.ActorID, error) {
	idAddr, err := sm.LookupIDAddress(ctx, addr, ts)
	if err != nil {
		return 0, xerrors.Errorf("state manager lookup id: %w", err)
	}
	id, err := address.IDFromAddress(idAddr)
	if err != nil {
		return 0, xerrors.Errorf("resolve actor id: id from addr: %w", err)
	}
	return abi.ActorID(id), nil
}

func (sm *StateManager) LookupRobustAddress(ctx context.Context, idAddr address.Address, ts *types.TipSet) (address.Address, error) {
	idAddrDecoded, err := address.IDFromAddress(idAddr)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to decode provided address as id addr: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.StateBlockstore())
	wrapStore := adt.WrapStore(ctx, cst)

	stateTree, err := state.LoadStateTree(cst, sm.parentState(ts))
	if err != nil {
		return address.Undef, xerrors.Errorf("load state tree: %w", err)
	}

	initActor, err := stateTree.GetActor(_init.Address)
	if err != nil {
		return address.Undef, xerrors.Errorf("load init actor: %w", err)
	}

	initState, err := _init.Load(wrapStore, initActor)
	if err != nil {
		return address.Undef, xerrors.Errorf("load init state: %w", err)
	}
	robustAddr := address.Undef

	err = initState.ForEachActor(func(id abi.ActorID, addr address.Address) error {
		if uint64(id) == idAddrDecoded {
			robustAddr = addr
			// Hacky way to early return from ForEach
			return xerrors.New("robust address found")
		}
		return nil
	})
	if robustAddr == address.Undef {
		if err == nil {
			return address.Undef, xerrors.Errorf("Address %s not found", idAddr.String())
		}
		return address.Undef, xerrors.Errorf("finding address: %w", err)
	}
	return robustAddr, nil
}

func (sm *StateManager) ValidateChain(ctx context.Context, ts *types.TipSet) error {
	tschain := []*types.TipSet{ts}
	for ts.Height() != 0 {
		next, err := sm.cs.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return err
		}

		tschain = append(tschain, next)
		ts = next
	}

	lastState := tschain[len(tschain)-1].ParentState()
	for i := len(tschain) - 1; i >= 0; i-- {
		cur := tschain[i]
		log.Infof("computing state (height: %d, ts=%s)", cur.Height(), cur.Cids())
		if cur.ParentState() != lastState {
			return xerrors.Errorf("tipset chain had state mismatch at height %d", cur.Height())
		}
		st, _, err := sm.TipSetState(ctx, cur)
		if err != nil {
			return err
		}
		lastState = st
	}

	return nil
}

func (sm *StateManager) SetVMConstructor(nvm func(context.Context, *vm.VMOpts) (vm.Interface, error)) {
	sm.newVM = nvm
}

func (sm *StateManager) VMConstructor() func(context.Context, *vm.VMOpts) (vm.Interface, error) {
	return func(ctx context.Context, opts *vm.VMOpts) (vm.Interface, error) {
		return sm.newVM(ctx, opts)
	}
}

func (sm *StateManager) GetNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version {
	// The epochs here are the _last_ epoch for every version, or -1 if the
	// version is disabled.
	for _, spec := range sm.networkVersions {
		if height <= spec.atOrBelow {
			return spec.networkVersion
		}
	}
	return sm.latestVersion
}

func (sm *StateManager) VMSys() vm.SyscallBuilder {
	return sm.Syscalls
}

func (sm *StateManager) GetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	pts, err := sm.ChainStore().GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	r := rand.NewStateRand(sm.ChainStore(), pts.Cids(), sm.beacon, sm.GetNetworkVersion)

	digest, err := r.GetBeaconRandomness(ctx, randEpoch)
	if err != nil {
		return nil, xerrors.Errorf("getting beacon randomness: %w", err)
	}

	ret, err := rand.DrawRandomnessFromDigest(digest, personalization, randEpoch, entropy)
	if err != nil {
		return nil, xerrors.Errorf("drawing beacon randomness: %w", err)
	}

	return ret, nil

}

func (sm *StateManager) GetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	pts, err := sm.ChainStore().LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset key: %w", err)
	}

	r := rand.NewStateRand(sm.ChainStore(), pts.Cids(), sm.beacon, sm.GetNetworkVersion)

	digest, err := r.GetChainRandomness(ctx, randEpoch)
	if err != nil {
		return nil, xerrors.Errorf("getting chain randomness: %w", err)
	}

	ret, err := rand.DrawRandomnessFromDigest(digest, personalization, randEpoch, entropy)
	if err != nil {
		return nil, xerrors.Errorf("drawing chain randomness: %w", err)
	}

	return ret, nil
}

func (sm *StateManager) GetRandomnessDigestFromBeacon(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) ([32]byte, error) {
	pts, err := sm.ChainStore().GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return [32]byte{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	r := rand.NewStateRand(sm.ChainStore(), pts.Cids(), sm.beacon, sm.GetNetworkVersion)
	return r.GetBeaconRandomness(ctx, randEpoch)
}

func (sm *StateManager) GetBeaconEntry(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) (*types.BeaconEntry, error) {
	pts, err := sm.ChainStore().GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	r := rand.NewStateRand(sm.ChainStore(), pts.Cids(), sm.beacon, sm.GetNetworkVersion)
	return r.GetBeaconEntry(ctx, randEpoch)
}

func (sm *StateManager) GetRandomnessDigestFromTickets(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) ([32]byte, error) {
	pts, err := sm.ChainStore().LoadTipSet(ctx, tsk)
	if err != nil {
		return [32]byte{}, xerrors.Errorf("loading tipset key: %w", err)
	}

	r := rand.NewStateRand(sm.ChainStore(), pts.Cids(), sm.beacon, sm.GetNetworkVersion)
	return r.GetChainRandomness(ctx, randEpoch)
}
