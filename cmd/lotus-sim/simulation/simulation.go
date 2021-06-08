package simulation

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"

	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
)

var log = logging.Logger("simulation")

const onboardingProjectionLookback = 2 * 7 * builtin.EpochsInDay // lookback two weeks

const (
	minPreCommitBatchSize   = 1
	maxPreCommitBatchSize   = miner5.PreCommitSectorBatchMaxSize
	minProveCommitBatchSize = 4
	maxProveCommitBatchSize = miner5.MaxAggregatedSectors
)

// config is the simulation's config, persisted to the local metadata store and loaded on start.
//
// See simulationState.loadConfig and simulationState.saveConfig.
type config struct {
	Upgrades map[network.Version]abi.ChainEpoch
}

// upgradeSchedule constructs an stmgr.StateManager upgrade schedule, overriding any network upgrade
// epochs as specified in the config.
func (c *config) upgradeSchedule() (stmgr.UpgradeSchedule, error) {
	upgradeSchedule := stmgr.DefaultUpgradeSchedule()
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
	*Node

	name   string
	config config
	sm     *stmgr.StateManager

	// head
	st   *state.StateTree
	head *types.TipSet

	// lazy-loaded state
	// access through `simState(ctx)` to load on-demand.
	state *simulationState
}

// loadConfig loads a simulation's config from the datastore. This must be called on startup and may
// be called to restore the config from-disk.
func (sim *Simulation) loadConfig() error {
	configBytes, err := sim.MetadataDS.Get(sim.key("config"))
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
	return sim.MetadataDS.Put(sim.key("config"), buf)
}

// stateTree returns the current state-tree for the current head, computing the tipset if necessary.
// The state-tree is cached until the head is changed.
func (sim *Simulation) stateTree(ctx context.Context) (*state.StateTree, error) {
	if sim.st == nil {
		st, _, err := sim.sm.TipSetState(ctx, sim.head)
		if err != nil {
			return nil, err
		}
		sim.st, err = sim.sm.StateTree(st)
		if err != nil {
			return nil, err
		}
	}
	return sim.st, nil
}

// Loads the simulation state. The state is memoized so this will be fast except the first time.
func (sim *Simulation) simState(ctx context.Context) (*simulationState, error) {
	if sim.state == nil {
		log.Infow("loading simulation")
		state, err := loadSimulationState(ctx, sim)
		if err != nil {
			return nil, xerrors.Errorf("failed to load simulation state: %w", err)
		}
		sim.state = state
		log.Infow("simulation loaded", "miners", len(sim.state.minerInfos))
	}

	return sim.state, nil
}

var simulationPrefix = datastore.NewKey("/simulation")

// key returns the the key in the form /simulation/<subkey>/<simulation-name>. For example,
// /simulation/head/default.
func (sim *Simulation) key(subkey string) datastore.Key {
	return simulationPrefix.ChildString(subkey).ChildString(sim.name)
}

// Load loads the simulation state. This will happen automatically on first use, but it can be
// useful to preload for timing reasons.
func (sim *Simulation) Load(ctx context.Context) error {
	_, err := sim.simState(ctx)
	return err
}

// GetHead returns the current simulation head.
func (sim *Simulation) GetHead() *types.TipSet {
	return sim.head
}

// GetNetworkVersion returns the current network version for the simulation.
func (sim *Simulation) GetNetworkVersion() network.Version {
	return sim.sm.GetNtwkVersion(context.TODO(), sim.head.Height())
}

// SetHead updates the current head of the simulation and stores it in the metadata store. This is
// called for every Simulation.Step.
func (sim *Simulation) SetHead(head *types.TipSet) error {
	if err := sim.MetadataDS.Put(sim.key("head"), head.Key().Bytes()); err != nil {
		return xerrors.Errorf("failed to store simulation head: %w", err)
	}
	sim.st = nil // we'll compute this on-demand.
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
	sm, err := stmgr.NewStateManagerWithUpgradeSchedule(sim.Chainstore, newUpgradeSchedule)
	if err != nil {
		return err
	}
	err = sim.saveConfig()
	if err != nil {
		return err
	}

	sim.sm = sm
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
