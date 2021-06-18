package simulation

import (
	"context"
	"io"
	"strings"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/filecoin-project/lotus/blockstore"
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
	Repo       repo.LockedRepo
	Blockstore blockstore.Blockstore
	MetadataDS datastore.Batching
	Chainstore *store.ChainStore
}

// OpenNode opens the local lotus node for writing. This will fail if the node is online.
func OpenNode(ctx context.Context, path string) (*Node, error) {
	var node Node
	r, err := repo.NewFS(path)
	if err != nil {
		return nil, err
	}

	node.Repo, err = r.Lock(repo.FullNode)
	if err != nil {
		_ = node.Close()
		return nil, err
	}

	node.Blockstore, err = node.Repo.Blockstore(ctx, repo.UniversalBlockstore)
	if err != nil {
		_ = node.Close()
		return nil, err
	}

	node.MetadataDS, err = node.Repo.Datastore(ctx, "/metadata")
	if err != nil {
		_ = node.Close()
		return nil, err
	}

	node.Chainstore = store.NewChainStore(node.Blockstore, node.Blockstore, node.MetadataDS, vm.Syscalls(mock.Verifier), nil)
	return &node, nil
}

// Close cleanly close the node. Please call this on shutdown to make sure everything is flushed.
func (nd *Node) Close() error {
	var err error
	if closer, ok := nd.Blockstore.(io.Closer); ok && closer != nil {
		err = multierr.Append(err, closer.Close())
	}
	if nd.MetadataDS != nil {
		err = multierr.Append(err, nd.MetadataDS.Close())
	}
	if nd.Repo != nil {
		err = multierr.Append(err, nd.Repo.Close())
	}
	return err
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
	sim.StateManager, err = stmgr.NewStateManagerWithUpgradeSchedule(nd.Chainstore, us)
	if err != nil {
		return nil, xerrors.Errorf("failed to create state manager for simulation %s: %w", name, err)
	}
	return sim, nil
}

// Create creates a new simulation.
//
// - This will fail if a simulation already exists with the given name.
// - Name must not contain a '/'.
func (nd *Node) CreateSim(ctx context.Context, name string, head *types.TipSet) (*Simulation, error) {
	if strings.Contains(name, "/") {
		return nil, xerrors.Errorf("simulation name %q cannot contain a '/'", name)
	}
	stages, err := stages.DefaultPipeline()
	if err != nil {
		return nil, err
	}
	sim := &Simulation{
		name:         name,
		Node:         nd,
		StateManager: stmgr.NewStateManager(nd.Chainstore),
		stages:       stages,
	}
	if has, err := nd.MetadataDS.Has(sim.key("head")); err != nil {
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
	items, err := nd.MetadataDS.Query(query.Query{
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
		err = multierr.Append(err, nd.MetadataDS.Delete(key))
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
		value, err := nd.MetadataDS.Get(key)
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
			err = nd.MetadataDS.Put(key, value)
		} else {
			err = nd.MetadataDS.Delete(key)
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
