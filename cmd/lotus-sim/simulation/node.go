package simulation

import (
	"context"
	"strings"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/mock"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/stages"
	"github.com/filecoin-project/lotus/node/repo"
)

// Node represents the local lotus node, or at least the part of it we care about.
type Node struct {
	repo       repo.LockedRepo
	Blockstore blockstore.Blockstore
	MetadataDS datastore.Batching
	Chainstore *store.ChainStore
}

// OpenNode opens the local lotus node for writing. This will fail if the node is online.
func OpenNode(ctx context.Context, path string) (*Node, error) {
	r, err := repo.NewFS(path)
	if err != nil {
		return nil, err
	}

	return NewNode(ctx, r)
}

// NewNode constructs a new node from the given repo.
func NewNode(ctx context.Context, r repo.Repo) (nd *Node, _err error) {
	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		return nil, err
	}
	defer func() {
		if _err != nil {
			_ = lr.Close()
		}
	}()

	bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
	if err != nil {
		return nil, err
	}

	ds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return nil, err
	}
	return &Node{
		repo:       lr,
		Chainstore: store.NewChainStore(bs, bs, ds, filcns.Weight, nil),
		MetadataDS: ds,
		Blockstore: bs,
	}, err
}

// Close cleanly close the repo. Please call this on shutdown to make sure everything is flushed.
func (nd *Node) Close() error {
	if nd.repo != nil {
		return nd.repo.Close()
	}
	return nil
}

// LoadSim loads
func (nd *Node) LoadSim(ctx context.Context, name string) (*Simulation, error) {
	stages, err := stages.DefaultPipeline()
	if err != nil {
		return nil, err
	}
	sim := &Simulation{
		Node:   nd,
		name:   name,
		stages: stages,
	}

	sim.head, err = sim.loadNamedTipSet("head")
	if err != nil {
		return nil, err
	}
	sim.start, err = sim.loadNamedTipSet("start")
	if err != nil {
		return nil, err
	}

	err = sim.loadConfig()
	if err != nil {
		return nil, xerrors.Errorf("failed to load config for simulation %s: %w", name, err)
	}

	us, err := sim.config.upgradeSchedule()
	if err != nil {
		return nil, xerrors.Errorf("failed to create upgrade schedule for simulation %s: %w", name, err)
	}
	sim.StateManager, err = stmgr.NewStateManager(nd.Chainstore, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(mock.Verifier), us,
		nil, nd.MetadataDS, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to create state manager for simulation %s: %w", name, err)
	}
	return sim, nil
}

// CreateSim creates a new simulation.
//
// - This will fail if a simulation already exists with the given name.
// - Num must not contain a '/'.
func (nd *Node) CreateSim(ctx context.Context, name string, head *types.TipSet) (*Simulation, error) {
	if strings.Contains(name, "/") {
		return nil, xerrors.Errorf("simulation name %q cannot contain a '/'", name)
	}
	stages, err := stages.DefaultPipeline()
	if err != nil {
		return nil, err
	}
	sm, err := stmgr.NewStateManager(nd.Chainstore, consensus.NewTipSetExecutor(filcns.RewardFunc),
		vm.Syscalls(mock.Verifier), filcns.DefaultUpgradeSchedule(), nil, nd.MetadataDS, nil)
	if err != nil {
		return nil, xerrors.Errorf("creating state manager: %w", err)
	}
	sim := &Simulation{
		name:         name,
		Node:         nd,
		StateManager: sm,
		stages:       stages,
	}
	if has, err := nd.MetadataDS.Has(ctx, sim.key("head")); err != nil {
		return nil, err
	} else if has {
		return nil, xerrors.Errorf("simulation named %s already exists", name)
	}

	if err := sim.storeNamedTipSet("start", head); err != nil {
		return nil, xerrors.Errorf("failed to set simulation start: %w", err)
	}

	if err := sim.SetHead(head); err != nil {
		return nil, err
	}

	return sim, nil
}

// ListSims lists all simulations.
func (nd *Node) ListSims(ctx context.Context) ([]string, error) {
	prefix := simulationPrefix.ChildString("head").String()
	items, err := nd.MetadataDS.Query(ctx, query.Query{
		Prefix:   prefix,
		KeysOnly: true,
		Orders:   []query.Order{query.OrderByKey{}},
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to list simulations: %w", err)
	}

	defer func() { _ = items.Close() }()

	var names []string
	for {
		select {
		case result, ok := <-items.Next():
			if !ok {
				return names, nil
			}
			if result.Error != nil {
				return nil, xerrors.Errorf("failed to retrieve next simulation: %w", result.Error)
			}
			names = append(names, strings.TrimPrefix(result.Key, prefix+"/"))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

var simFields = []string{"head", "start", "config"}

// DeleteSim deletes a simulation and all related metadata.
//
// NOTE: This function does not delete associated messages, blocks, or chain state.
func (nd *Node) DeleteSim(ctx context.Context, name string) error {
	var err error
	for _, field := range simFields {
		key := simulationPrefix.ChildString(field).ChildString(name)
		err = multierr.Append(err, nd.MetadataDS.Delete(ctx, key))
	}
	return err
}

// CopySim copies a simulation.
func (nd *Node) CopySim(ctx context.Context, oldName, newName string) error {
	if strings.Contains(newName, "/") {
		return xerrors.Errorf("simulation name %q cannot contain a '/'", newName)
	}
	if strings.Contains(oldName, "/") {
		return xerrors.Errorf("simulation name %q cannot contain a '/'", oldName)
	}

	values := make(map[string][]byte)
	for _, field := range simFields {
		key := simulationPrefix.ChildString(field).ChildString(oldName)
		value, err := nd.MetadataDS.Get(ctx, key)
		if err == datastore.ErrNotFound {
			continue
		} else if err != nil {
			return err
		}
		values[field] = value
	}

	if _, ok := values["head"]; !ok {
		return xerrors.Errorf("simulation named %s not found", oldName)
	}

	for _, field := range simFields {
		key := simulationPrefix.ChildString(field).ChildString(newName)
		var err error
		if value, ok := values[field]; ok {
			err = nd.MetadataDS.Put(ctx, key, value)
		} else {
			err = nd.MetadataDS.Delete(ctx, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// RenameSim renames a simulation.
func (nd *Node) RenameSim(ctx context.Context, oldName, newName string) error {
	if err := nd.CopySim(ctx, oldName, newName); err != nil {
		return err
	}
	return nd.DeleteSim(ctx, oldName)
}
