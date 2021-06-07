package simulation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/build"
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

type config struct {
	Upgrades map[network.Version]abi.ChainEpoch
}

func (c *config) upgradeSchedule() (stmgr.UpgradeSchedule, error) {
	upgradeSchedule := stmgr.DefaultUpgradeSchedule()
	expected := make(map[network.Version]struct{}, len(c.Upgrades))
	for nv := range c.Upgrades {
		expected[nv] = struct{}{}
	}
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
	if len(expected) > 0 {
		missing := make([]network.Version, 0, len(expected))
		for nv := range expected {
			missing = append(missing, nv)
		}
		return nil, xerrors.Errorf("unknown network versions %v in config", missing)
	}
	return newUpgradeSchedule, nil
}

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

func (sim *Simulation) key(subkey string) datastore.Key {
	return simulationPrefix.ChildString(subkey).ChildString(sim.name)
}

// Load loads the simulation state. This will happen automatically on first use, but it can be
// useful to preload for timing reasons.
func (sim *Simulation) Load(ctx context.Context) error {
	_, err := sim.simState(ctx)
	return err
}

func (sim *Simulation) GetHead() *types.TipSet {
	return sim.head
}

func (sim *Simulation) SetHead(head *types.TipSet) error {
	if err := sim.MetadataDS.Put(sim.key("head"), head.Key().Bytes()); err != nil {
		return xerrors.Errorf("failed to store simulation head: %w", err)
	}
	sim.st = nil // we'll compute this on-demand.
	sim.head = head
	return nil
}

func (sim *Simulation) Name() string {
	return sim.name
}

func (sim *Simulation) postChainCommitInfo(ctx context.Context, epoch abi.ChainEpoch) (abi.Randomness, error) {
	commitRand, err := sim.Chainstore.GetChainRandomness(
		ctx, sim.head.Cids(), crypto.DomainSeparationTag_PoStChainCommit, epoch, nil, true)
	return commitRand, err
}

const beaconPrefix = "mockbeacon:"

func (sim *Simulation) nextBeaconEntries() []types.BeaconEntry {
	parentBeacons := sim.head.Blocks()[0].BeaconEntries
	lastBeacon := parentBeacons[len(parentBeacons)-1]
	beaconRound := lastBeacon.Round + 1

	buf := make([]byte, len(beaconPrefix)+8)
	copy(buf, beaconPrefix)
	binary.BigEndian.PutUint64(buf[len(beaconPrefix):], beaconRound)
	beaconRand := sha256.Sum256(buf)
	return []types.BeaconEntry{{
		Round: beaconRound,
		Data:  beaconRand[:],
	}}
}

func (sim *Simulation) nextTicket() *types.Ticket {
	newProof := sha256.Sum256(sim.head.MinTicket().VRFProof)
	return &types.Ticket{
		VRFProof: newProof[:],
	}
}

func (sim *Simulation) makeTipSet(ctx context.Context, messages []*types.Message) (*types.TipSet, error) {
	parentTs := sim.head
	parentState, parentRec, err := sim.sm.TipSetState(ctx, parentTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute parent tipset: %w", err)
	}
	msgsCid, err := sim.storeMessages(ctx, messages)
	if err != nil {
		return nil, xerrors.Errorf("failed to store block messages: %w", err)
	}

	uts := parentTs.MinTimestamp() + build.BlockDelaySecs

	blks := []*types.BlockHeader{{
		Miner:                 parentTs.MinTicketBlock().Miner, // keep reusing the same miner.
		Ticket:                sim.nextTicket(),
		BeaconEntries:         sim.nextBeaconEntries(),
		Parents:               parentTs.Cids(),
		Height:                parentTs.Height() + 1,
		ParentStateRoot:       parentState,
		ParentMessageReceipts: parentRec,
		Messages:              msgsCid,
		ParentBaseFee:         baseFee,
		Timestamp:             uts,
		ElectionProof:         &types.ElectionProof{WinCount: 1},
	}}
	err = sim.Chainstore.PersistBlockHeaders(blks...)
	if err != nil {
		return nil, xerrors.Errorf("failed to persist block headers: %w", err)
	}
	newTipSet, err := types.NewTipSet(blks)
	if err != nil {
		return nil, xerrors.Errorf("failed to create new tipset: %w", err)
	}
	now := time.Now()
	_, _, err = sim.sm.TipSetState(ctx, newTipSet)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute new tipset: %w", err)
	}
	duration := time.Since(now)
	log.Infow("computed tipset", "duration", duration, "height", newTipSet.Height())

	return newTipSet, nil
}

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

func (sim *Simulation) saveConfig() error {
	buf, err := json.Marshal(sim.config)
	if err != nil {
		return err
	}
	return sim.MetadataDS.Put(sim.key("config"), buf)
}
