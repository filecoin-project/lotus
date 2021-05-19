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
	"github.com/filecoin-project/lotus/node/repo"
)

type Node struct {
	Repo       repo.LockedRepo
	Blockstore blockstore.Blockstore
	MetadataDS datastore.Batching
	Chainstore *store.ChainStore
}

func OpenNode(ctx context.Context, path string) (*Node, error) {
	var node Node
	r, err := repo.NewFS(path)
	if err != nil {
		return nil, err
	}

	node.Repo, err = r.Lock(repo.FullNode)
	if err != nil {
		node.Close()
		return nil, err
	}

	node.Blockstore, err = node.Repo.Blockstore(ctx, repo.UniversalBlockstore)
	if err != nil {
		node.Close()
		return nil, err
	}

	node.MetadataDS, err = node.Repo.Datastore(ctx, "/metadata")
	if err != nil {
		node.Close()
		return nil, err
	}

	node.Chainstore = store.NewChainStore(node.Blockstore, node.Blockstore, node.MetadataDS, vm.Syscalls(mockVerifier{}), nil)
	return &node, nil
}

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

func (nd *Node) LoadSim(ctx context.Context, name string) (*Simulation, error) {
	sim := &Simulation{
		Node: nd,
		name: name,
	}
	tskBytes, err := nd.MetadataDS.Get(sim.key("head"))
	if err != nil {
		return nil, xerrors.Errorf("failed to load simulation %s: %w", name, err)
	}
	tsk, err := types.TipSetKeyFromBytes(tskBytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse simulation %s's tipset %v: %w", name, tskBytes, err)
	}
	sim.head, err = nd.Chainstore.LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load simulation tipset %s: %w", tsk, err)
	}

	err = sim.loadConfig()
	if err != nil {
		return nil, xerrors.Errorf("failed to load config for simulation %s: %w", name, err)
	}

	us, err := sim.config.upgradeSchedule()
	if err != nil {
		return nil, xerrors.Errorf("failed to create upgrade schedule for simulation %s: %w", name, err)
	}
	sim.sm, err = stmgr.NewStateManagerWithUpgradeSchedule(nd.Chainstore, us)
	if err != nil {
		return nil, xerrors.Errorf("failed to create state manager for simulation %s: %w", name, err)
	}
	return sim, nil
}

func (nd *Node) CreateSim(ctx context.Context, name string, head *types.TipSet) (*Simulation, error) {
	if strings.Contains(name, "/") {
		return nil, xerrors.Errorf("simulation name %q cannot contain a '/'", name)
	}
	sim := &Simulation{
		name: name,
		Node: nd,
		sm:   stmgr.NewStateManager(nd.Chainstore),
	}
	if has, err := nd.MetadataDS.Has(sim.key("head")); err != nil {
		return nil, err
	} else if has {
		return nil, xerrors.Errorf("simulation named %s already exists", name)
	}

	if err := sim.SetHead(head); err != nil {
		return nil, err
	}

	return sim, nil
}

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
	defer items.Close()
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

func (nd *Node) DeleteSim(ctx context.Context, name string) error {
	// TODO: make this a bit more generic?
	keys := []datastore.Key{
		simulationPrefix.ChildString("head").ChildString(name),
		simulationPrefix.ChildString("config").ChildString(name),
	}
	var err error
	for _, key := range keys {
		err = multierr.Append(err, nd.MetadataDS.Delete(key))
	}
	return err
}
