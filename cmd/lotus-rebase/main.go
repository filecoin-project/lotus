package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
)

// FullAPI is a JSON-RPC client targeting a full node. It's initialized in a
// cli.BeforeFunc.
var FullAPI v0api.FullNode

// Closer is the closer for the JSON-RPC client, which must be called on
// cli.AfterFunc.
var Closer jsonrpc.ClientCloser

var log = logging.Logger("lotus-rebase")

const DefaultLotusRepoPath = "~/.lotus"

var repoFlag = cli.StringFlag{
	Name:      "repo",
	EnvVars:   []string{"LOTUS_PATH"},
	Value:     DefaultLotusRepoPath,
	TakesFile: true,
}

func main() {
	adjustLogLevels()

	app := &cli.App{
		Name:    "lotus-rebase",
		Usage:   "lotus-rebase is a tool for replaying chain history on top of an alternative network version and report differences",
		Version: build.UserVersion(),
		Flags: []cli.Flag{
			&repoFlag,
			&cli.StringFlag{
				Name:  "log-level",
				Value: "info",
			},
			&cli.StringFlag{
				Name:     "epoch",
				Usage:    "the epoch (1234), or range of epochs (1212..1234; both inclusive), to replay",
				Required: true,
			},
			&cli.Uint64Flag{
				Name:     "onto-nv",
				Usage:    "network version to rebase chain activity onto",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "adapt-gas",
				Usage: "whether to re-estimate gas on an OOG",
				Value: "no",
			},
		},
		Before: func(cctx *cli.Context) error {
			return logging.SetLogLevel("lotus-rebase", cctx.String("log-level"))
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%+v", err)
		os.Exit(1)
		return
	}
}

func adjustLogLevels() {
	_ = logging.SetLogLevel("*", "INFO")
	_ = logging.SetLogLevelRegex("badger*", "ERROR")
	_ = logging.SetLogLevel("drand", "ERROR")
	_ = logging.SetLogLevel("chainstore", "ERROR")
}

func run(c *cli.Context) error {
	const (
		AdaptGasNo = "no"
		AdaptGasUp = "up"
	)

	var (
		epochStart int
		epochEnd   int
		err        error

		ctx = c.Context
	)

	bs, mds, closeFn, err := openStores(c)
	if err != nil {
		return fmt.Errorf("failed to openStores: %w", err)
	}
	defer closeFn()

	// Parameter: epoch
	switch splt := strings.Split(c.String("epoch"), ".."); {
	case len(splt) == 1:
		if epochStart, err = strconv.Atoi(splt[0]); err != nil {
			return fmt.Errorf("failed to parse single epoch: %w", err)
		}
		epochEnd = epochStart
	case len(splt) == 2:
		if epochStart, err = strconv.Atoi(splt[0]); err != nil {
			return fmt.Errorf("failed to parse start epoch: %w", err)
		}
		if epochEnd, err = strconv.Atoi(splt[1]); err != nil {
			return fmt.Errorf("failed to parse end epoch: %w", err)
		}
		if epochEnd < epochStart {
			return fmt.Errorf("end epoch smaller (%d) than start epoch (%d)", epochEnd, epochStart)
		}
	default:
		return fmt.Errorf("invalid epoch format; expected single epoch (1212), or range (1212..1234); got: %s", c.String("epoch"))
	}

	// Get a seed chainstore and state manager so we can inspect the chain first
	// and situate ourselves.
	cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
	if err := cs.Load(ctx); err != nil {
		return fmt.Errorf("failed to load chainstore: %w", err)
	}

	sm, err := stmgr.NewStateManager(cs,
		filcns.NewTipSetExecutor(),
		vm.Syscalls(ffiwrapper.ProofVerifier),
		filcns.DefaultUpgradeSchedule(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}

	var (
		nvStart = sm.GetNetworkVersion(ctx, abi.ChainEpoch(epochStart))
		nvEnd   = sm.GetNetworkVersion(ctx, abi.ChainEpoch(epochEnd))
		// Parameter: onto-nv
		nvTgt = network.Version(c.Uint64("onto-nv"))
	)

	if nvStart != nvEnd {
		return fmt.Errorf("network version at start epoch (%d) not equal to network version at end epoch (%d); this tool is not sophisticated enough yet to deal with such a case", nvStart, nvEnd)
	}

	idxCurr := slices.IndexFunc(filcns.DefaultUpgradeSchedule(), func(upgrade stmgr.Upgrade) bool {
		return upgrade.Network == nvStart
	})
	idxTgt := slices.IndexFunc(filcns.DefaultUpgradeSchedule(), func(upgrade stmgr.Upgrade) bool {
		return upgrade.Network == nvTgt
	})
	if idxTgt == -1 {
		return fmt.Errorf("some network versions don't have migrations associated to them")
	}
	upgradePlan := filcns.DefaultUpgradeSchedule()[idxCurr+1 : idxTgt+1]

	var (
		tse      = filcns.NewTipSetExecutor()
		syscalls = vm.Syscalls(ffiwrapper.ProofVerifier)
	)

	drand, err := setupDrand(ctx, cs)
	if err != nil {
		return fmt.Errorf("failed to setup drand: %w", err)
	}

	for i := epochStart; i <= epochEnd; i++ {
		color.Cyan("processing inclusion epoch %d", i)

		execTs, err := cs.GetTipsetByHeight(ctx, abi.ChainEpoch(i+1), nil, false)
		if err != nil {
			return fmt.Errorf("failed to get execution tipset of messages included at epoch %d: %w", i, err)
		}
		incTs, err := cs.GetTipsetByHeight(ctx, abi.ChainEpoch(i), execTs, false)
		if err != nil {
			return fmt.Errorf("failed to get inclusion tipset at epoch %d: %w", i, err)
		}

		c, _ := incTs.Key().Cid()
		color.Yellow("  inclusion tipset (epoch %d): %s", incTs.Height(), c)
		c, _ = execTs.Key().Cid()
		color.Yellow("  execution tipset (epoch %d): %s", execTs.Height(), c)

		msgs, err := cs.MessagesForTipset(ctx, incTs)
		if err != nil {
			return fmt.Errorf("failed to get messages for tipset: %w", err)
		}

		baseFee := incTs.Blocks()[0].ParentBaseFee

		color.Yellow("  message count: %d", len(msgs))
		color.Yellow("  base fee: %d", baseFee)

		// This is very wrong, but the ChainAPI has higher-level functions we want to use
		// and their implementations don't depend on anything else but the ChainStore.
		receipts, err := (&full.ChainAPI{Chain: cs}).ChainGetParentReceipts(ctx, execTs.MinTicketBlock().Cid())
		if err != nil {
			return fmt.Errorf("failed to get receipts of messages included in epoch %d: %w", incTs.Height(), err)
		}

		color.Yellow("  receipt count: %d", len(receipts))

		var (
			overlayBs = blockstore.NewBuffered(bs)
			// TODO overlayMds; currently writes can go to the _actual_ metadata store.
			tmpCs = store.NewChainStore(overlayBs, overlayBs, mds, filcns.Weight, nil)
		)

		aggMig := func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, oldState cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (root cid.Cid, err error) {
			root = oldState
			for _, m := range upgradePlan {
				if root, err = m.Migration(ctx, sm, cache, cb, root, height, ts); err != nil {
					return cid.Undef, err
				}
			}
			return root, nil
		}

		upSched := stmgr.UpgradeSchedule{
			stmgr.Upgrade{
				Height:        incTs.Height(),
				Network:       nvTgt,
				Migration:     aggMig,
				PreMigrations: nil,
			},
		}

		tmpSm, err := stmgr.NewStateManager(tmpCs, tse, syscalls, upSched, drand)
		if err != nil {
			return fmt.Errorf("failed to setup state manager: %w", err)
		}

		r := rand.NewStateRand(tmpCs, incTs.Cids(), drand, sm.GetNetworkVersion)

		bmsgs, err := getBlockMessages(ctx, sm, incTs)
		if err != nil {
			return fmt.Errorf("failed to get block messages: %w", err)
		}

		_, postRecRoot, err := tse.ApplyBlocks(ctx, tmpSm, incTs.Height(), incTs.ParentState(), bmsgs, incTs.Height(), r, nil, false, baseFee, incTs)
		if err != nil {
			return fmt.Errorf("failed to apply blocks: %w", err)
		}

		compareReceipts(ctx, tmpCs, receipts, postRecRoot, msgs)
	}

	return nil
}

func compareReceipts(ctx context.Context, cs *store.ChainStore, oldReceipts []*types.MessageReceipt, newReceiptsRoot cid.Cid, msgs []types.ChainMsg) error {
	newReceipts, err := loadReceipts(ctx, cs, newReceiptsRoot)
	if err != nil {
		return fmt.Errorf("failed to load receipts: %w", err)
	}

	if len(oldReceipts) != len(newReceipts) {
		return fmt.Errorf("length of receipts didn't match; %d (orig) != %d (new)", len(oldReceipts), len(newReceipts))
	}

	for i, prev := range oldReceipts {
		new_ := newReceipts[i]
		var failures []string
		if new_.ExitCode != prev.ExitCode {
			failures = append(failures, fmt.Sprintf("exit code mismatch: %d (new) != %d (prev)", new_.ExitCode, prev.ExitCode))
		}
		if new_.GasUsed != prev.GasUsed {
			failures = append(failures, fmt.Sprintf("gas used mismatch: %d (new) != %d (prev) [%+.4f%%]", new_.GasUsed, prev.GasUsed, (float64(new_.GasUsed-prev.GasUsed)/float64(prev.GasUsed))*100))
		}
		if !slices.Equal(new_.Return, prev.Return) {
			failures = append(failures, fmt.Sprintf("return mismatch: %s (new) != %s (prev)", hex.EncodeToString(new_.Return), hex.EncodeToString(prev.Return)))
		}
		if new_.EventsRoot != prev.EventsRoot {
			failures = append(failures, fmt.Sprintf("events root mismatch: %s (new) != %s (prev)", new_.EventsRoot, prev.EventsRoot))
			// Once again, this construction voodoo is not right and is highly coupled with the internals of ChainAPI.
			newEvs, err := (&full.ChainAPI{ExposedBlockstore: cs.StateBlockstore()}).ChainGetEvents(ctx, *new_.EventsRoot)
			if err != nil {
				failures = append(failures, fmt.Sprintf("failed to load new events: %s", err))
				goto Failures
			}
			oldEvs, err := (&full.ChainAPI{ExposedBlockstore: cs.StateBlockstore()}).ChainGetEvents(ctx, *prev.EventsRoot)
			if err != nil {
				failures = append(failures, fmt.Sprintf("failed to load old events: %s", err))
				goto Failures
			}
			if !reflect.DeepEqual(newEvs, oldEvs) {
				failures = append(failures, fmt.Sprintf("(!) events ARE NOT not equivalent"))
			} else {
				failures = append(failures, fmt.Sprintf("although events ARE equivalent"))
			}
		}

	Failures:
		if len(failures) == 0 {
			color.Green("  [%d] (%s): RECEIPTS EQUAL", i, msgs[i].Cid())
		} else {
			color.Red("  [%d] (%s): RECEIPTS MISMATCH", i, msgs[i].Cid())
			for _, f := range failures {
				color.Red("    - " + f)
			}
		}
	}
	return nil
}

func openStores(c *cli.Context) (blockstore.Blockstore, datastore.Batching, func() error, error) {
	var (
		fsrepo *repo.FsRepo
		lkrepo repo.LockedRepo
		bs     blockstore.Blockstore
		err    error
	)

	if fsrepo, err = repo.NewFS(c.String("repo")); err != nil {
		return nil, nil, nil, err
	}

	if lkrepo, err = fsrepo.Lock(repo.FullNode); err != nil {
		return nil, nil, nil, err
	}

	if bs, err = lkrepo.Blockstore(c.Context, repo.UniversalBlockstore); err != nil {
		_ = lkrepo.Close()
		return nil, nil, nil, fmt.Errorf("failed to open blockstore: %w", err)
	}

	mds, err := lkrepo.Datastore(c.Context, "/metadata")
	if err != nil {
		_ = lkrepo.Close()
		return nil, nil, nil, err
	}

	return bs, mds, lkrepo.Close, nil
}

func setupDrand(ctx context.Context, cs *store.ChainStore) (beacon.Schedule, error) {
	gen, err := cs.GetGenesis(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get genesis: %w", err)
	}

	var shd beacon.Schedule
	for _, dc := range build.DrandConfigSchedule() {
		bc, err := drand.NewDrandBeacon(gen.Timestamp, build.BlockDelaySecs, nil, dc.Config)
		if err != nil {
			return nil, fmt.Errorf("creating drand beacon: %w", err)
		}
		shd = append(shd, beacon.BeaconPoint{Start: dc.Start, Beacon: bc})
	}
	return shd, nil
}

func getBlockMessages(ctx context.Context, sm *stmgr.StateManager, ts *types.TipSet) ([]filcns.FilecoinBlockMessages, error) {
	blkmsgs, err := sm.ChainStore().BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return nil, fmt.Errorf("getting block messages for tipset: %w", err)
	}

	bms := make([]filcns.FilecoinBlockMessages, len(blkmsgs))
	for i := range bms {
		bms[i].BlockMessages = blkmsgs[i]
		bms[i].WinCount = ts.Blocks()[i].ElectionProof.WinCount
	}
	return bms, nil
}

func loadReceipts(ctx context.Context, cs *store.ChainStore, root cid.Cid) ([]types.MessageReceipt, error) {
	a, err := blockadt.AsArray(cs.ActorStore(ctx), root)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	ret := make([]types.MessageReceipt, 0, a.Length())
	var r types.MessageReceipt
	a.ForEach(&r, func(i int64) error {
		ret = append(ret, r)
		return nil
	})
	return ret, nil
}
