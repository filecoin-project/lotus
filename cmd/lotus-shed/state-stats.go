package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/docker/go-units"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
)

type actorStats struct {
	Address address.Address
	Actor   *types.Actor
	Fields  []fieldItem
	Stats   api.ObjStat
}

type fieldItem struct {
	Name  string
	Cid   cid.Cid
	Stats api.ObjStat
}

type cacheNodeGetter struct {
	ds    format.NodeGetter
	cache *lru.TwoQueueCache[cid.Cid, format.Node]
}

func newCacheNodeGetter(d format.NodeGetter, size int) (*cacheNodeGetter, error) {
	cng := &cacheNodeGetter{ds: d}

	cache, err := lru.New2Q[cid.Cid, format.Node](size)
	if err != nil {
		return nil, err
	}

	cng.cache = cache

	return cng, nil
}

func (cng *cacheNodeGetter) Get(ctx context.Context, c cid.Cid) (format.Node, error) {
	if n, ok := cng.cache.Get(c); ok {
		return n, nil
	}

	n, err := cng.ds.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	cng.cache.Add(c, n)

	return n, nil
}

func (cng *cacheNodeGetter) GetMany(ctx context.Context, list []cid.Cid) <-chan *format.NodeOption {
	out := make(chan *format.NodeOption, len(list))
	go func() {
		for _, c := range list {
			n, err := cng.Get(ctx, c)
			if err != nil {
				out <- &format.NodeOption{Err: err}
				continue
			}

			out <- &format.NodeOption{Node: n}
		}
	}()

	return out
}

type dagStatCollector struct {
	ds   format.NodeGetter
	walk func(format.Node) ([]*format.Link, error)

	statsLk sync.Mutex
	stats   api.ObjStat
}

func (dsc *dagStatCollector) record(ctx context.Context, nd format.Node) error {
	size, err := nd.Size()
	if err != nil {
		return err
	}

	dsc.statsLk.Lock()
	defer dsc.statsLk.Unlock()

	dsc.stats.Size = dsc.stats.Size + size
	dsc.stats.Links = dsc.stats.Links + 1

	return nil
}

func (dsc *dagStatCollector) walkLinks(ctx context.Context, c cid.Cid) ([]*format.Link, error) {
	nd, err := dsc.ds.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	if err := dsc.record(ctx, nd); err != nil {
		return nil, err
	}

	return dsc.walk(nd)
}

type ChainStoreTipSetResolver struct {
	Chain *store.ChainStore
}

func (tsr *ChainStoreTipSetResolver) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return tsr.Chain.GetHeaviestTipSet(), nil
}

func (tsr *ChainStoreTipSetResolver) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := tsr.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return tsr.Chain.GetTipsetByHeight(ctx, h, ts, true)
}
func (tsr *ChainStoreTipSetResolver) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return tsr.Chain.LoadTipSet(ctx, tsk)
}

var statObjCmd = &cli.Command{
	Name:  "stat-obj",
	Usage: "calculates the size of any DAG in the blockstore",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		c, err := cid.Parse(cctx.Args().First())
		if err != nil {
			return err
		}

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
		dsc := &dagStatCollector{
			ds:   dag,
			walk: carWalkFunc,
		}

		if err := merkledag.Walk(ctx, dsc.walkLinks, c, cid.NewSet().Visit, merkledag.Concurrent()); err != nil {
			return err
		}

		return DumpJSON(dsc.stats)
	},
}

var statActorCmd = &cli.Command{
	Name:  "stat-actor",
	Usage: "calculates the size of actors and their immeidate structures",
	Description: `Any DAG linked by the actor object (field) will have its size calculated independently of all
other linked DAG. If an actor has two fields containing links to the same DAG the structure size will be counted
twice, included in each fields size individually.

The top level stats reported for an actor is computed independently of all fields and is a more accurate
accounting of the true size of the actor in the state datastore.

The calculation of these stats results in the actor state being traversed twice. The dag-cache-size flag can be used
to reduce the number of decode operations performed by caching the decoded object after first access.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
		&cli.IntFlag{
			Name:  "workers",
			Usage: "number of workers to use when processing",
			Value: 10,
		},
		&cli.IntFlag{
			Name:  "dag-cache-size",
			Usage: "cache size per worker (setting to 0 disables)",
			Value: 8092,
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "process all actors in stateroot of tipset",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "pretty",
			Usage: "print formated output instead of ldjson",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		var addrs []address.Address

		if !cctx.Bool("all") {
			for _, a := range cctx.Args().Slice() {
				addr, err := address.NewFromString(a)
				if err != nil {
					return err
				}

				addrs = append(addrs, addr)
			}
		}

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		mds, err := lr.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, nil, nil)
		if err := cs.Load(ctx); err != nil {
			return nil
		}

		tsExec := consensus.NewTipSetExecutor(filcns.RewardFunc)
		sm, err := stmgr.NewStateManager(cs, tsExec, vm.Syscalls(ffiwrapper.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil, mds)
		if err != nil {
			return err
		}

		tsr := &ChainStoreTipSetResolver{
			Chain: cs,
		}

		ts, err := lcli.LoadTipSet(ctx, cctx, tsr)
		if err != nil {
			return err
		}

		log.Infow("tipset", "parentstate", ts.ParentState())

		if len(addrs) == 0 && cctx.Bool("all") {
			var err error
			addrs, err = sm.ListAllActors(ctx, ts)
			if err != nil {
				return err
			}
		}

		numWorkers := cctx.Int("workers")
		dagCacheSize := cctx.Int("dag-cache-size")

		eg, egctx := errgroup.WithContext(ctx)

		jobs := make(chan address.Address, numWorkers)
		results := make(chan actorStats, numWorkers)

		worker := func(ctx context.Context, id int) error {
			completed := 0
			defer func() {
				log.Infow("worker done", "id", id, "completed", completed)
			}()

			for {
				select {
				case addr, ok := <-jobs:
					if !ok {
						return nil
					}

					actor, err := sm.LoadActor(ctx, addr, ts)
					if err != nil {
						return err
					}

					var dag format.NodeGetter = merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
					if dagCacheSize != 0 {
						var err error
						dag, err = newCacheNodeGetter(merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs))), dagCacheSize)
						if err != nil {
							return err
						}
					}

					actStats, err := collectStats(ctx, addr, actor, dag)
					if err != nil {
						return err
					}

					select {
					case results <- actStats:
					case <-ctx.Done():
						return ctx.Err()
					}
				case <-ctx.Done():
					return ctx.Err()
				}

				completed = completed + 1
			}
		}

		for w := 0; w < numWorkers; w++ {
			id := w
			eg.Go(func() error {
				return worker(egctx, id)
			})
		}

		go func() {
			defer close(jobs)
			for _, addr := range addrs {
				jobs <- addr
			}
		}()

		go func() {
			// error is check later
			eg.Wait() //nolint:errcheck
			close(results)
		}()

		for {
			select {
			case result, ok := <-results:
				if !ok {
					return eg.Wait()
				}

				if cctx.Bool("pretty") {
					DumpStats(result)
				} else {
					if err := DumpJSON(result); err != nil {
						return err
					}
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	},
}

func collectStats(ctx context.Context, addr address.Address, actor *types.Actor, dag format.NodeGetter) (actorStats, error) {
	log.Infow("actor", "addr", addr, "code", actor.Code, "name", builtin.ActorNameByCode(actor.Code))

	nd, err := dag.Get(ctx, actor.Head)
	if err != nil {
		return actorStats{}, err
	}

	// When it comes to fvm / evm actors this method of inspecting fields will probably not work
	// and we may only be able to collect stats for the top level object. We might be able to iterate
	// over the top level fields for the actors and identify field that are CIDs, but unsure if we would
	// be able to identify a field name.

	oif, err := vm.DumpActorState(consensus.NewTipSetExecutor(filcns.RewardFunc).NewActorRegistry(), actor, nd.RawData())
	if err != nil {
		oif = nil
	}

	fields := []fieldItem{}

	// Account actors return nil from DumpActorState as they have no state
	if oif != nil {
		v := reflect.Indirect(reflect.ValueOf(oif))
		for i := 0; i < v.NumField(); i++ {
			varName := v.Type().Field(i).Name
			varType := v.Type().Field(i).Type
			varValue := v.Field(i).Interface()

			if varType == reflect.TypeOf(cid.Cid{}) {
				fields = append(fields, fieldItem{
					Name: varName,
					Cid:  varValue.(cid.Cid),
				})
			}
		}
	}

	actStats := actorStats{
		Address: addr,
		Actor:   actor,
	}

	dsc := &dagStatCollector{
		ds:   dag,
		walk: carWalkFunc,
	}

	if err := merkledag.Walk(ctx, dsc.walkLinks, actor.Head, cid.NewSet().Visit, merkledag.Concurrent()); err != nil {
		return actorStats{}, err
	}

	actStats.Stats = dsc.stats

	for _, field := range fields {
		dsc := &dagStatCollector{
			ds:   dag,
			walk: carWalkFunc,
		}

		if err := merkledag.Walk(ctx, dsc.walkLinks, field.Cid, cid.NewSet().Visit, merkledag.Concurrent()); err != nil {
			return actorStats{}, err
		}

		field.Stats = dsc.stats

		actStats.Fields = append(actStats.Fields, field)
	}

	return actStats, nil
}

func DumpJSON(i interface{}) error {
	bs, err := json.Marshal(i)
	if err != nil {
		return err
	}

	fmt.Println(string(bs))

	return nil
}

func DumpStats(actStats actorStats) {
	strtype := builtin.ActorNameByCode(actStats.Actor.Code)
	fmt.Printf("Address:\t%s\n", actStats.Address)
	fmt.Printf("Balance:\t%s\n", types.FIL(actStats.Actor.Balance))
	fmt.Printf("Nonce:\t\t%d\n", actStats.Actor.Nonce)
	fmt.Printf("Code:\t\t%s (%s)\n", actStats.Actor.Code, strtype)
	fmt.Printf("Head:\t\t%s\n", actStats.Actor.Head)
	fmt.Println()

	fmt.Printf("%-*s%-*s%-*s\n", 32, "Field", 24, "Size", 24, "\"Blocks\"")

	stats := actStats.Stats
	sizeStr := units.BytesSize(float64(stats.Size))
	fmt.Printf("%-*s%-*s%-*s%-*d\n", 32, "<self>", 10, sizeStr, 14, fmt.Sprintf("(%d)", stats.Size), 24, stats.Links)

	for _, s := range actStats.Fields {
		stats := s.Stats
		sizeStr := units.BytesSize(float64(stats.Size))
		fmt.Printf("%-*s%-*s%-*s%-*d\n", 32, s.Name, 10, sizeStr, 14, fmt.Sprintf("(%d)", stats.Size), 24, stats.Links)
	}

	fmt.Println("--------------------------------------------------------------------------")
}
