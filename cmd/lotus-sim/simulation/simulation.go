package simulation

import (
	"context"
	"encoding/json"
	"runtime"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/mock"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/stages"
)

var log = logging.Logger("simulation")

// config is the simulation's config, persisted to the local metadata store and loaded on start.
//
// See Simulation.loadConfig and Simulation.saveConfig.
type config struct {
	Upgrades map[network.Version]abi.ChainEpoch
}

// upgradeSchedule constructs an stmgr.StateManager upgrade schedule, overriding any network upgrade
// epochs as specified in the config.
func (c *config) upgradeSchedule() (stmgr.UpgradeSchedule, error) {
	upgradeSchedule := filcns.DefaultUpgradeSchedule()
	expected := make(map[network.Version]struct{}, len(c.Upgrades))
	for nv := range c.Upgrades {
		expected[nv] = struct{}{}
	}

	// Update network upgrade epochs.
	newUpgradeSchedule := upgradeSchedule[:0]
	for _, upgrade := range upgradeSchedule {
		if height, ok := c.Upgrades[upgrade.Network]; ok {
			delete(expected, upgrade.Network)
			if height < 0 {
				continue
			}
			upgrade.Height = height
		}
		newUpgradeSchedule = append(newUpgradeSchedule, upgrade)
	}

	// Make sure we didn't try to configure an unknown network version.
	if len(expected) > 0 {
		missing := make([]network.Version, 0, len(expected))
		for nv := range expected {
			missing = append(missing, nv)
		}
		return nil, xerrors.Errorf("unknown network versions %v in config", missing)
	}

	// Finally, validate it. This ensures we don't change the order of the upgrade or anything
	// like that.
	if err := newUpgradeSchedule.Validate(); err != nil {
		return nil, err
	}
	return newUpgradeSchedule, nil
}

// Simulation specifies a lotus-sim simulation.
type Simulation struct {
	Node         *Node
	StateManager *stmgr.StateManager

	name   string
	config config
	start  *types.TipSet

	// head
	head *types.TipSet

	stages []stages.Stage
}

// loadConfig loads a simulation's config from the datastore. This must be called on startup and may
// be called to restore the config from-disk.
func (sim *Simulation) loadConfig() error {
	configBytes, err := sim.Node.MetadataDS.Get(context.TODO(), sim.key("config"))
	if err == nil {
		err = json.Unmarshal(configBytes, &sim.config)
	}
	switch err {
	case nil:
	case datastore.ErrNotFound:
		sim.config = config{}
	default:
		return xerrors.Errorf("failed to load config: %w", err)
	}
	return nil
}

// saveConfig saves the current config to the datastore. This must be called whenever the config is
// changed.
func (sim *Simulation) saveConfig() error {
	buf, err := json.Marshal(sim.config)
	if err != nil {
		return err
	}
	return sim.Node.MetadataDS.Put(context.TODO(), sim.key("config"), buf)
}

var simulationPrefix = datastore.NewKey("/simulation")

// key returns the key in the form /simulation/<subkey>/<simulation-name>. For example,
// /simulation/head/default.
func (sim *Simulation) key(subkey string) datastore.Key {
	return simulationPrefix.ChildString(subkey).ChildString(sim.name)
}

// loadNamedTipSet the tipset with the given name (for this simulation)
func (sim *Simulation) loadNamedTipSet(name string) (*types.TipSet, error) {
	tskBytes, err := sim.Node.MetadataDS.Get(context.TODO(), sim.key(name))
	if err != nil {
		return nil, xerrors.Errorf("failed to load tipset %s/%s: %w", sim.name, name, err)
	}
	tsk, err := types.TipSetKeyFromBytes(tskBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse tipste %v (%s/%s): %w", tskBytes, sim.name, name, err)
	}
	ts, err := sim.Node.Chainstore.LoadTipSet(context.TODO(), tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load tipset %s (%s/%s): %w", tsk, sim.name, name, err)
	}
	return ts, nil
}

// storeNamedTipSet stores the tipset at name (relative to the simulation).
func (sim *Simulation) storeNamedTipSet(name string, ts *types.TipSet) error {
	if err := sim.Node.MetadataDS.Put(context.TODO(), sim.key(name), ts.Key().Bytes()); err != nil {
		return xerrors.Errorf("failed to store tipset (%s/%s): %w", sim.name, name, err)
	}
	return nil
}

// GetHead returns the current simulation head.
func (sim *Simulation) GetHead() *types.TipSet {
	return sim.head
}

// GetStart returns simulation's parent tipset.
func (sim *Simulation) GetStart() *types.TipSet {
	return sim.start
}

// GetNetworkVersion returns the current network version for the simulation.
func (sim *Simulation) GetNetworkVersion() network.Version {
	return sim.StateManager.GetNetworkVersion(context.TODO(), sim.head.Height())
}

// SetHead updates the current head of the simulation and stores it in the metadata store. This is
// called for every Simulation.Step.
func (sim *Simulation) SetHead(head *types.TipSet) error {
	if err := sim.storeNamedTipSet("head", head); err != nil {
		return err
	}
	sim.head = head
	return nil
}

// Name returns the simulation's name.
func (sim *Simulation) Name() string {
	return sim.name
}

// SetUpgradeHeight sets the height of the given network version change (and saves the config).
//
// This fails if the specified epoch has already passed or the new upgrade schedule is invalid.
func (sim *Simulation) SetUpgradeHeight(nv network.Version, epoch abi.ChainEpoch) (_err error) {
	if epoch <= sim.head.Height() {
		return xerrors.Errorf("cannot set upgrade height in the past (%d <= %d)", epoch, sim.head.Height())
	}

	if sim.config.Upgrades == nil {
		sim.config.Upgrades = make(map[network.Version]abi.ChainEpoch, 1)
	}

	sim.config.Upgrades[nv] = epoch
	defer func() {
		if _err != nil {
			// try to restore the old config on error.
			_ = sim.loadConfig()
		}
	}()

	newUpgradeSchedule, err := sim.config.upgradeSchedule()
	if err != nil {
		return err
	}
	sm, err := stmgr.NewStateManager(sim.Node.Chainstore, consensus.NewTipSetExecutor(filcns.RewardFunc),
		vm.Syscalls(mock.Verifier), newUpgradeSchedule, nil, sim.Node.MetadataDS, nil)
	if err != nil {
		return err
	}
	err = sim.saveConfig()
	if err != nil {
		return err
	}

	sim.StateManager = sm
	return nil
}

// ListUpgrades returns any future network upgrades.
func (sim *Simulation) ListUpgrades() (stmgr.UpgradeSchedule, error) {
	upgrades, err := sim.config.upgradeSchedule()
	if err != nil {
		return nil, err
	}
	var pending stmgr.UpgradeSchedule
	for _, upgrade := range upgrades {
		if upgrade.Height < sim.head.Height() {
			continue
		}
		pending = append(pending, upgrade)
	}
	return pending, nil
}

type AppliedMessage struct {
	types.Message
	types.MessageReceipt
}

// Walk walks the simulation's chain from the current head back to the first tipset.
func (sim *Simulation) Walk(
	ctx context.Context,
	lookback int64,
	cb func(sm *stmgr.StateManager,
		ts *types.TipSet,
		stCid cid.Cid,
		messages []*AppliedMessage) error,
) error {
	store := sim.Node.Chainstore.ActorStore(ctx)
	minEpoch := sim.start.Height()
	if lookback != 0 {
		minEpoch = sim.head.Height() - abi.ChainEpoch(lookback)
	}

	// Given that loading messages and receipts can be a little bit slow, we do this in parallel.
	//
	// 1. We spin up some number of workers.
	// 2. We hand tipsets to workers in round-robin order.
	// 3. We pull "resolved" tipsets in the same round-robin order.
	// 4. We serially call the callback in reverse-chain order.
	//
	// We have a buffer of size 1 for both resolved tipsets and unresolved tipsets. This should
	// ensure that we never block unnecessarily.

	type work struct {
		ts     *types.TipSet
		stCid  cid.Cid
		recCid cid.Cid
	}
	type result struct {
		ts       *types.TipSet
		stCid    cid.Cid
		messages []*AppliedMessage
	}

	// This is more disk bound than CPU bound, but eh...
	workerCount := runtime.NumCPU() * 2

	workQs := make([]chan *work, workerCount)
	resultQs := make([]chan *result, workerCount)

	for i := range workQs {
		workQs[i] = make(chan *work, 1)
	}

	for i := range resultQs {
		resultQs[i] = make(chan *result, 1)
	}

	grp, ctx := errgroup.WithContext(ctx)

	// Walk the chain and fire off work items.
	grp.Go(func() error {
		ts := sim.head
		stCid, recCid, err := sim.StateManager.TipSetState(ctx, ts)
		if err != nil {
			return err
		}
		i := 0
		for ts.Height() > minEpoch {
			if err := ctx.Err(); err != nil {
				return ctx.Err()
			}

			select {
			case workQs[i] <- &work{ts, stCid, recCid}:
			case <-ctx.Done():
				return ctx.Err()
			}

			stCid = ts.MinTicketBlock().ParentStateRoot
			recCid = ts.MinTicketBlock().ParentMessageReceipts
			ts, err = sim.Node.Chainstore.LoadTipSet(ctx, ts.Parents())
			if err != nil {
				return xerrors.Errorf("loading parent: %w", err)
			}
			i = (i + 1) % workerCount
		}
		for _, q := range workQs {
			close(q)
		}
		return nil
	})

	// Spin up one worker per queue pair.
	for i := 0; i < workerCount; i++ {
		workQ := workQs[i]
		resultQ := resultQs[i]
		grp.Go(func() error {
			for {
				if err := ctx.Err(); err != nil {
					return ctx.Err()
				}

				var job *work
				var ok bool
				select {
				case job, ok = <-workQ:
				case <-ctx.Done():
					return ctx.Err()
				}

				if !ok {
					break
				}

				msgs, err := sim.Node.Chainstore.MessagesForTipset(ctx, job.ts)
				if err != nil {
					return err
				}

				recs, err := blockadt.AsArray(store, job.recCid)
				if err != nil {
					return xerrors.Errorf("amt load: %w", err)
				}
				applied := make([]*AppliedMessage, len(msgs))
				var rec types.MessageReceipt
				err = recs.ForEach(&rec, func(i int64) error {
					applied[i] = &AppliedMessage{
						Message:        *msgs[i].VMMessage(),
						MessageReceipt: rec,
					}
					return nil
				})
				if err != nil {
					return err
				}
				select {
				case resultQ <- &result{
					ts:       job.ts,
					stCid:    job.stCid,
					messages: applied,
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			close(resultQ)
			return nil
		})
	}

	// Process results in the same order we enqueued them.
	grp.Go(func() error {
		qs := resultQs
		for len(qs) > 0 {
			newQs := qs[:0]
			for _, q := range qs {
				if err := ctx.Err(); err != nil {
					return ctx.Err()
				}
				select {
				case r, ok := <-q:
					if !ok {
						continue
					}
					err := cb(sim.StateManager, r.ts, r.stCid, r.messages)
					if err != nil {
						return err
					}
				case <-ctx.Done():
					return ctx.Err()
				}
				newQs = append(newQs, q)
			}
			qs = newQs
		}
		return nil
	})

	// Wait for everything to finish.
	return grp.Wait()
}
