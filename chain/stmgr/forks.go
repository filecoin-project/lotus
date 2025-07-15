package stmgr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v8/actors/migration/nv16"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// EnvDisablePreMigrations when set to '1' stops pre-migrations from running
const EnvDisablePreMigrations = "LOTUS_DISABLE_PRE_MIGRATIONS"

// MigrationCache can be used to cache information used by a migration. This is primarily useful to
// "pre-compute" some migration state ahead of time, and make it accessible in the migration itself.
type MigrationCache interface {
	Write(key string, value cid.Cid) error
	Read(key string) (bool, cid.Cid, error)
	Load(key string, loadFunc func() (cid.Cid, error)) (cid.Cid, error)
}

// MigrationFunc is a migration function run at every upgrade.
//
//   - The cache is a per-upgrade cache, pre-populated by pre-migrations.
//   - The oldState is the state produced by the upgrade epoch.
//   - The returned newState is the new state that will be used by the next epoch.
//   - The height is the upgrade epoch height (already executed).
//   - The tipset is the first non-null tipset after the upgrade height (the tipset in
//     which the upgrade is executed). Do not assume that ts.Height() is the upgrade height.
//
// NOTE: In StateCompute and CallWithGas, the passed tipset is actually the tipset _before_ the
// upgrade. The tipset should really only be used for referencing the "current chain".
type MigrationFunc func(
	ctx context.Context,
	sm *StateManager, cache MigrationCache,
	cb ExecMonitor,
	oldState cid.Cid,
	height abi.ChainEpoch, ts *types.TipSet,
) (newState cid.Cid, err error)

// PreMigrationFunc is a function run _before_ a network upgrade to pre-compute part of the network
// upgrade and speed it up.
type PreMigrationFunc func(
	ctx context.Context,
	sm *StateManager, cache MigrationCache,
	oldState cid.Cid,
	height abi.ChainEpoch, ts *types.TipSet,
) error

// PreMigration describes a pre-migration step to prepare for a network state upgrade. Pre-migrations
// are optimizations, are not guaranteed to run, and may be canceled and/or run multiple times.
type PreMigration struct {
	// PreMigration is the pre-migration function to run at the specified time. This function is
	// run asynchronously and must abort promptly when canceled.
	PreMigration PreMigrationFunc

	// StartWithin specifies that this pre-migration should be started at most StartWithin
	// epochs before the upgrade.
	StartWithin abi.ChainEpoch

	// DontStartWithin specifies that this pre-migration should not be started DontStartWithin
	// epochs before the final upgrade epoch.
	//
	// This should be set such that the pre-migration is likely to complete before StopWithin.
	DontStartWithin abi.ChainEpoch

	// StopWithin specifies that this pre-migration should be stopped StopWithin epochs of the
	// final upgrade epoch.
	StopWithin abi.ChainEpoch
}

type Upgrade struct {
	Height    abi.ChainEpoch
	Network   network.Version
	Expensive bool
	Migration MigrationFunc

	// PreMigrations specifies a set of pre-migration functions to run at the indicated epochs.
	// These functions should fill the given cache with information that can speed up the
	// eventual full migration at the upgrade epoch.
	PreMigrations []PreMigration
}

type UpgradeSchedule []Upgrade

func (us UpgradeSchedule) Validate() error {
	// Make sure each upgrade is valid.
	for _, u := range us {
		if u.Network <= 0 {
			return xerrors.Errorf("cannot upgrade to version <= 0: %d", u.Network)
		}

		for _, m := range u.PreMigrations {
			if m.StartWithin <= 0 {
				return xerrors.Errorf("pre-migration must specify a positive start-within epoch")
			}

			if m.DontStartWithin < 0 || m.StopWithin < 0 {
				return xerrors.Errorf("pre-migration must specify non-negative epochs")
			}

			if m.StartWithin <= m.StopWithin {
				return xerrors.Errorf("pre-migration start-within must come before stop-within")
			}

			// If we have a don't-start-within.
			if m.DontStartWithin != 0 {
				if m.DontStartWithin < m.StopWithin {
					return xerrors.Errorf("pre-migration don't-start-within must come before stop-within")
				}
				if m.StartWithin <= m.DontStartWithin {
					return xerrors.Errorf("pre-migration start-within must come after don't-start-within")
				}
			}
		}
		if !sort.SliceIsSorted(u.PreMigrations, func(i, j int) bool {
			return u.PreMigrations[i].StartWithin > u.PreMigrations[j].StartWithin //nolint:scopelint,gosec
		}) {
			return xerrors.Errorf("pre-migrations must be sorted by start epoch")
		}
	}

	// Make sure the upgrade order makes sense.
	for i := 1; i < len(us); i++ {
		prev := &us[i-1]
		curr := &us[i]
		if !(prev.Network <= curr.Network) {
			return xerrors.Errorf("cannot downgrade from version %d to version %d", prev.Network, curr.Network)
		}
		// Make sure the heights make sense.
		if prev.Height < 0 {
			// Previous upgrade was disabled.
			continue
		}
		if !(prev.Height < curr.Height) {
			return xerrors.Errorf("upgrade heights must be strictly increasing: upgrade %d was at height %d, followed by upgrade %d at height %d", i-1, prev.Height, i, curr.Height)
		}
	}
	return nil
}

func (us UpgradeSchedule) GetNtwkVersion(e abi.ChainEpoch) (network.Version, error) {
	// Traverse from newest to oldest returning upgrade active during epoch e
	for i := len(us) - 1; i >= 0; i-- {
		u := us[i]
		// u.Height is the last epoch before u.Network becomes the active version
		if u.Height < e {
			return u.Network, nil
		}
	}

	return buildconstants.GenesisNetworkVersion, nil
}

func (sm *StateManager) HandleStateForks(ctx context.Context, root cid.Cid, height abi.ChainEpoch, cb ExecMonitor, ts *types.TipSet) (cid.Cid, error) {
	retCid := root
	u := sm.stateMigrations[height]
	if u != nil && u.upgrade != nil {
		migCid, ok, err := u.migrationResultCache.Get(ctx, root)
		if err == nil {
			if ok {
				log.Infow("CACHED migration", "height", height, "from", root, "to", migCid)
				foundMigratedRoot, err := sm.ChainStore().StateBlockstore().Has(ctx, migCid)
				if err != nil {
					log.Errorw("failed to check whether previous migration result is present", "err", err)
				} else if !foundMigratedRoot {
					log.Errorw("cached migration result not found in blockstore, running migration again")
					u.migrationResultCache.Delete(ctx, root)
				} else {
					return migCid, nil
				}
			}
		} else if !errors.Is(err, datastore.ErrNotFound) {
			log.Errorw("failed to lookup previous migration result", "err", err)
		} else {
			log.Debug("no cached migration found, migrating from scratch")
		}

		startTime := time.Now()
		log.Warnw("STARTING migration", "height", height, "from", root)
		// Yes, we clone the cache, even for the final upgrade epoch. Why? Reverts. We may
		// have to migrate multiple times.
		tmpCache := u.cache.Clone()
		retCid, err = u.upgrade(ctx, sm, tmpCache, cb, root, height, ts)
		if err != nil {
			log.Errorw("FAILED migration", "height", height, "from", root, "error", err)
			return cid.Undef, err
		}
		log.Warnw("COMPLETED migration",
			"height", height,
			"from", root,
			"to", retCid,
			"duration", time.Since(startTime),
		)

		// Only set if migration ran, we do not want a root => root mapping
		if err := u.migrationResultCache.Store(ctx, root, retCid); err != nil {
			log.Errorw("failed to store migration result", "err", err)
		}
	}

	return retCid, nil
}

// HasExpensiveForkBetween returns true where executing tipsets between the specified heights would
// trigger an expensive migration. NOTE: migrations occurring _at_ the target height are not
// included, as they're executed _after_ the target height.
func (sm *StateManager) HasExpensiveForkBetween(parent, height abi.ChainEpoch) bool {
	for h := parent; h < height; h++ {
		if _, ok := sm.expensiveUpgrades[h]; ok {
			return true
		}
	}
	return false
}

func runPreMigration(ctx context.Context, sm *StateManager, fn PreMigrationFunc, cache *nv16.MemMigrationCache, ts *types.TipSet) {
	height := ts.Height()
	parent := ts.ParentState()

	if disabled := os.Getenv(EnvDisablePreMigrations); strings.TrimSpace(disabled) == "1" {
		log.Warnw("SKIPPING pre-migration", "height", height)
		return
	}

	startTime := time.Now()

	log.Warn("STARTING pre-migration")
	// Clone the cache so we don't actually _update_ it
	// till we're done. Otherwise, if we fail, the next
	// migration to use the cache may assume that
	// certain blocks exist, even if they don't.
	tmpCache := cache.Clone()
	err := fn(ctx, sm, tmpCache, parent, height, ts)
	if err != nil {
		log.Errorw("FAILED pre-migration", "error", err)
		return
	}
	// Finally, if everything worked, update the cache.
	cache.Update(tmpCache)
	log.Warnw("COMPLETED pre-migration", "duration", time.Since(startTime))
}

func (sm *StateManager) preMigrationWorker(ctx context.Context) {
	defer close(sm.shutdown)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type op struct {
		after    abi.ChainEpoch
		notAfter abi.ChainEpoch
		run      func(ts *types.TipSet)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	// Turn each pre-migration into an operation in a schedule.
	var schedule []op
	for upgradeEpoch, migration := range sm.stateMigrations {
		cache := migration.cache
		for _, prem := range migration.preMigrations {
			preCtx, preCancel := context.WithCancel(ctx)
			migrationFunc := prem.PreMigration

			afterEpoch := upgradeEpoch - prem.StartWithin
			notAfterEpoch := upgradeEpoch - prem.DontStartWithin
			stopEpoch := upgradeEpoch - prem.StopWithin
			// We can't start after we stop.
			if notAfterEpoch > stopEpoch {
				notAfterEpoch = stopEpoch - 1
			}

			// Add an op to start a pre-migration.
			schedule = append(schedule, op{
				after:    afterEpoch,
				notAfter: notAfterEpoch,

				// TODO: are these values correct?
				run: func(ts *types.TipSet) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						runPreMigration(preCtx, sm, migrationFunc, cache, ts)
					}()
				},
			})

			// Add an op to cancel the pre-migration if it's still running.
			schedule = append(schedule, op{
				after:    stopEpoch,
				notAfter: -1,
				run:      func(ts *types.TipSet) { preCancel() },
			})
		}
	}

	// Then sort by epoch.
	sort.Slice(schedule, func(i, j int) bool {
		return schedule[i].after < schedule[j].after
	})

	// Finally, when the head changes, see if there's anything we need to do.
	//
	// We're intentionally ignoring reorgs as they don't matter for our purposes.
	for change := range sm.cs.SubHeadChanges(ctx) {
		for _, head := range change {
			for len(schedule) > 0 {
				op := &schedule[0]
				if head.Val.Height() < op.after {
					break
				}

				// If we haven't passed the pre-migration height...
				if op.notAfter < 0 || head.Val.Height() < op.notAfter {
					op.run(head.Val)
				}
				schedule = schedule[1:]
			}
		}
	}
}

func DoTransfer(tree types.StateTree, from, to address.Address, amt abi.TokenAmount, cb func(trace types.ExecutionTrace)) error {
	fromAct, err := tree.GetActor(from)
	if err != nil {
		return xerrors.Errorf("failed to get 'from' actor for transfer: %w", err)
	}

	fromAct.Balance = types.BigSub(fromAct.Balance, amt)
	if fromAct.Balance.Sign() < 0 {
		return xerrors.Errorf("(sanity) deducted more funds from target account than it had (%s, %s)", from, types.FIL(amt))
	}

	if err := tree.SetActor(from, fromAct); err != nil {
		return xerrors.Errorf("failed to persist from actor: %w", err)
	}

	toAct, err := tree.GetActor(to)
	if err != nil {
		return xerrors.Errorf("failed to get 'to' actor for transfer: %w", err)
	}

	toAct.Balance = types.BigAdd(toAct.Balance, amt)

	if err := tree.SetActor(to, toAct); err != nil {
		return xerrors.Errorf("failed to persist to actor: %w", err)
	}

	if cb != nil {
		// record the transfer in execution traces

		cb(types.ExecutionTrace{
			Msg: types.MessageTrace{
				From:  from,
				To:    to,
				Value: amt,
			},
		})
	}

	return nil
}

func TerminateActor(ctx context.Context, tree *state.StateTree, addr address.Address, em ExecMonitor, epoch abi.ChainEpoch, ts *types.TipSet) error {
	a, err := tree.GetActor(addr)
	if errors.Is(err, types.ErrActorNotFound) {
		return types.ErrActorNotFound
	} else if err != nil {
		return xerrors.Errorf("failed to get actor to delete: %w", err)
	}

	var trace types.ExecutionTrace
	if err := DoTransfer(tree, addr, builtin.BurntFundsActorAddr, a.Balance, func(t types.ExecutionTrace) {
		trace = t
	}); err != nil {
		return xerrors.Errorf("transferring terminated actor's balance: %w", err)
	}

	if em != nil {
		// record the transfer in execution traces

		fakeMsg := MakeFakeMsg(builtin.SystemActorAddr, addr, big.Zero(), uint64(epoch))

		if err := em.MessageApplied(ctx, ts, fakeMsg.Cid(), fakeMsg, &vm.ApplyRet{
			MessageReceipt: *MakeFakeRct(),
			ActorErr:       nil,
			ExecutionTrace: trace,
			Duration:       0,
			GasCosts:       nil,
		}, false); err != nil {
			return xerrors.Errorf("recording transfers: %w", err)
		}
	}

	err = tree.DeleteActor(addr)
	if err != nil {
		return xerrors.Errorf("deleting actor from tree: %w", err)
	}

	ia, err := tree.GetActor(init_.Address)
	if err != nil {
		return xerrors.Errorf("loading init actor: %w", err)
	}

	ias, err := init_.Load(&state.AdtStore{IpldStore: tree.Store}, ia)
	if err != nil {
		return xerrors.Errorf("loading init actor state: %w", err)
	}

	if err := ias.Remove(addr); err != nil {
		return xerrors.Errorf("deleting entry from address map: %w", err)
	}

	nih, err := tree.Store.Put(ctx, ias)
	if err != nil {
		return xerrors.Errorf("writing new init actor state: %w", err)
	}

	ia.Head = nih

	return tree.SetActor(init_.Address, ia)
}

func SetNetworkName(ctx context.Context, store adt.Store, tree *state.StateTree, name string) error {
	ia, err := tree.GetActor(init_.Address)
	if err != nil {
		return xerrors.Errorf("getting init actor: %w", err)
	}

	initState, err := init_.Load(store, ia)
	if err != nil {
		return xerrors.Errorf("reading init state: %w", err)
	}

	if err := initState.SetNetworkName(name); err != nil {
		return xerrors.Errorf("setting network name: %w", err)
	}

	ia.Head, err = store.Put(ctx, initState)
	if err != nil {
		return xerrors.Errorf("writing new init state: %w", err)
	}

	if err := tree.SetActor(init_.Address, ia); err != nil {
		return xerrors.Errorf("setting init actor: %w", err)
	}

	return nil
}

func MakeKeyAddr(splitAddr address.Address, count uint64) (address.Address, error) {
	var b bytes.Buffer
	if err := splitAddr.MarshalCBOR(&b); err != nil {
		return address.Undef, xerrors.Errorf("marshalling split address: %w", err)
	}

	if err := binary.Write(&b, binary.BigEndian, count); err != nil {
		return address.Undef, xerrors.Errorf("writing count into a buffer: %w", err)
	}

	if err := binary.Write(&b, binary.BigEndian, []byte("Ignition upgrade")); err != nil {
		return address.Undef, xerrors.Errorf("writing fork name into a buffer: %w", err)
	}

	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		return address.Undef, xerrors.Errorf("create actor address: %w", err)
	}

	return addr, nil
}

func MakeFakeMsg(from address.Address, to address.Address, amt abi.TokenAmount, nonce uint64) *types.Message {
	return &types.Message{
		From:  from,
		To:    to,
		Value: amt,
		Nonce: nonce,
	}
}

func MakeFakeRct() *types.MessageReceipt {
	return &types.MessageReceipt{
		ExitCode: 0,
		Return:   nil,
		GasUsed:  0,
	}
}
