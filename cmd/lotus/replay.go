package main

import (
	"bytes"
	"context"
	"fmt"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
	metricsi "github.com/ipfs/go-metrics-interface"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"net/http"
)

var replayCmd = &cli.Command{
	Name:  "replay",
	Usage: "replay chain data to dst repo",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "src-repo",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "dst-repo",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "src-tail-ts",
			Required: true,
			Usage:    "the dst-tipset in src repo",
		},
		&cli.IntFlag{
			Name:     "start-height",
			Required: true,
		},
		&cli.IntFlag{
			Name:     "end-height",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		var components struct {
			fx.In
			CS  *store.ChainStore
			Stm *stmgr.StateManager
		}

		stopper, err := node.New(ctx, Reject(cctx, &components))
		defer stopper(ctx)
		if err != nil {
			return err
		}

		r, err := repo.NewFS(cctx.String("src-repo"))
		if err != nil {
			return fmt.Errorf("opening fs repo: %w", err)
		}

		err = r.Init(repo.FullNode)
		if err != nil && err != repo.ErrRepoExists {
			return fmt.Errorf("src repo error: %w", err)
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		bs, err := lr.Blockstore(cctx.Context, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		mds, err := lr.Datastore(context.TODO(), "/metadata")
		if err != nil {
			return err
		}

		j, err := fsjournal.OpenFSJournal(lr, journal.EnvDisabledEvents())
		if err != nil {
			return fmt.Errorf("failed to open journal: %w", err)
		}

		cst := store.NewChainStore(bs, bs, mds, filcns.Weight, j)
		cids, err := lcli.ParseTipSetString(cctx.String("src-tail-ts"))
		if err != nil {
			return err
		}

		ts, err := cst.LoadTipSet(ctx, types.NewTipSetKey(cids...))
		if err != nil {
			return fmt.Errorf("importing chain failed: %w", err)
		}

		start := cctx.Int("start-height")
		end := cctx.Int("end-height")

		log.Info("start, end height", start, end)

		tss := []*types.TipSet{}

		for ts.Height() >= abi.ChainEpoch(start) {
			if ts.Height() <= abi.ChainEpoch(end) {
				tss = append(tss, ts)
			}
			ts, err = cst.LoadTipSet(ctx, ts.Parents())
			if err != nil {
				return fmt.Errorf("load ts failed: %w", err)
			}
		}

		got := len(tss)
		for i := 0; i < got/2; i++ {
			j := got - i - 1
			tss[i], tss[j] = tss[j], tss[i]
		}

		//replay
		for _, ts := range tss {
			_, ires, err := components.Stm.ExecutionTrace(ctx, ts)
			if err != nil {
				log.Error(err)
				return err
			}

			log.Infof("execute tipset %v, len(ires): %v", ts.Height(), len(ires))

			//validate
			for _, r := range ires {
				events, err := LoadEvents(ctx, components.CS, *r.MsgRct.EventsRoot)
				if err != nil {
					log.Errorf("load events for root %v failed: %v", r.MsgRct.EventsRoot.String(), err)
					return err
				}

				log.Infof("load events for root %v at %v successfly, events: %v", r.MsgRct.EventsRoot.String(), ts.Height(), events)
			}
		}

		return nil
	},
}

func Reject(cctx *cli.Context, target ...interface{}) node.Option {
	return node.Options(
		ContextModule(context.Background()),
		StateManager(),
		InjectChainRepo(cctx), OfflineDataSource(),
	)
}

type GlobalContext context.Context

func ContextModule(ctx context.Context) node.Option {
	return node.Options(node.Override(new(GlobalContext), ctx),
		node.Override(new(*http.ServeMux), http.NewServeMux()),
		node.Override(new(helpers.MetricsCtx), metricsi.CtxScope(ctx, "bell")))
}

func StateManager() node.Option {
	return node.Options(node.Override(new(vm.SyscallBuilder), vm.Syscalls),
		node.Override(new(storiface.Verifier), ffiwrapper.ProofVerifier),
		node.Override(new(journal.Journal), modules.OpenFilesystemJournal),
		node.Override(new(journal.DisabledEvents), journal.EnvDisabledEvents),
		node.Override(new(store.WeightFunc), filcns.Weight),
		node.Override(new(*store.ChainStore), modules.ChainStore),
		node.Override(new(stmgr.Executor), filcns.NewTipSetExecutor),
		node.Override(new(dtypes.DrandSchedule), modules.BuiltinDrandConfig),
		node.Override(new(dtypes.AfterGenesisSet), modules.SetGenesis),
		node.Override(new(beacon.Schedule), modules.RandomSchedule),
		node.Override(new(stmgr.UpgradeSchedule), modules.UpgradeSchedule),
		node.Override(new(*stmgr.StateManager), stmgr.NewStateManager),
		node.Override(new(modules.Genesis), modules.LoadGenesis(build.MaybeGenesis())),
		node.Override(new(dtypes.ChainBlockstore), node.From(new(dtypes.BasicChainBlockstore))),
		node.Override(new(dtypes.StateBlockstore), node.From(new(dtypes.BasicChainBlockstore))),
		node.Override(new(dtypes.BaseBlockstore), node.From(new(dtypes.BasicChainBlockstore))),
		node.Override(new(dtypes.ExposedBlockstore), node.From(new(dtypes.BasicChainBlockstore))))
}

func InjectChainRepo(cctx *cli.Context) node.Option {
	return node.Override(new(repo.LockedRepo), func(lc fx.Lifecycle) repo.LockedRepo {
		r, err := repo.NewFS(cctx.String("dst-repo"))
		if err != nil {
			panic(fmt.Errorf("opening fs repo: %w", err))
		}
		err = r.Init(repo.FullNode)
		if err != nil && err != repo.ErrRepoExists {
			panic(fmt.Errorf("dst repo error: %w", err))
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			panic(fmt.Errorf("lock repo failed: %w", err))
		}

		return modules.LockedRepo(lr)(lc)
	})
}

func OfflineDataSource() node.Option {
	return node.Options(
		// Notice: we may need to use other datastore someday. It depends on
		// the origin data structs.
		node.Override(new(dtypes.AfterGenesisSet), modules.SetGenesis),
		node.Override(new(dtypes.BasicChainBlockstore), modules.UniversalBlockstore),
		node.Override(new(dtypes.MetadataDS), modules.Datastore(true)),
	)
}

func LoadEvents(ctx context.Context, cs *store.ChainStore, eventsRoot cid.Cid) ([]types.Event, error) {
	store := cs.ActorStore(ctx)
	amt, err := amt4.LoadAMT(ctx, store, eventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	if err != nil {
		return nil, err
	}

	ret := make([]types.Event, 0, amt.Len())
	err = amt.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
		var evt types.Event
		if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
			return err
		}
		ret = append(ret, evt)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return ret, nil
}
