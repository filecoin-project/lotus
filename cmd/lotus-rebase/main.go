package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
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

var log = logging.Logger("lotus-rebase")

const DefaultLotusRepoPath = "~/.lotus"

var repoFlag = cli.StringFlag{
	Name:      "repo",
	EnvVars:   []string{"LOTUS_PATH"},
	Value:     DefaultLotusRepoPath,
	TakesFile: true,
}

func main() {
	_ = os.Setenv("RUST_BACKTRACE", "1")

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
			&cli.Uint64Flag{
				Name:  "bump-gas-factor",
				Usage: "factor by which to bump gas to retry an OOG",
				Value: 10,
			},
			&cli.BoolFlag{
				Name:  "omit-pre-gas-bump-reporting",
				Usage: "whether to omit reporting for errors pre gas bump",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "traces",
				Usage: "dumps a trace in CWD for every message with exit code (except OOG), return data, or events mismatches",
				Value: true,
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
	_ = logging.SetLogLevel("statemgr", "ERROR")
	_ = logging.SetLogLevel("fil-consensus", "ERROR")
}

func run(cctx *cli.Context) error {
	var (
		epochStart int
		epochEnd   int
		err        error

		omitReportingPreBump = cctx.Bool("omit-pre-gas-bump-reporting")
		tracing              = cctx.Bool("traces")

		ctx = cctx.Context
	)

	bs, mds, closeFn, err := openStores(cctx)
	if err != nil {
		return fmt.Errorf("failed to openStores: %w", err)
	}
	defer closeFn() //nolint

	// Parameter: epoch
	switch splt := strings.Split(cctx.String("epoch"), ".."); {
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
		return fmt.Errorf("invalid epoch format; expected single epoch (1212), or range (1212..1234); got: %s", cctx.String("epoch"))
	}

	// Get a seed chainstore and state manager so we can inspect the chain first
	// and situate ourselves.
	cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
	if err := cs.Load(ctx); err != nil {
		return fmt.Errorf("failed to load chainstore: %w", err)
	}
	cs.StoreEvents(true)

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
		nvTgt = network.Version(cctx.Uint64("onto-nv"))
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

	for i := epochStart; i <= epochEnd; {
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
			overlayBs = blockstore.NewTieredBstore(bs, blockstore.NewMemorySync())

			// TODO overlayMds; currently writes can go to the _actual_ metadata store.
			tmpCs = store.NewChainStore(overlayBs, overlayBs, mds, filcns.Weight, nil)
		)
		tmpCs.StoreEvents(true)

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

		var traces []*api.InvocResult
		tracer := stmgr.InvocationTracer{Trace: &traces}
		_, postRecRoot, err := tse.ApplyBlocks(ctx, tmpSm, incTs.Height(), incTs.ParentState(), bmsgs, execTs.Height(), r, &tracer, true, baseFee, incTs)
		if err != nil {
			return fmt.Errorf("failed to apply blocks: %w", err)
		}

		bump, out, err := compareReceipts(ctx, tmpCs, receipts, postRecRoot, msgs, tracing, traces)
		if err != nil {
			return fmt.Errorf("failed to compare receipts: %w", err)
		}

		bumpRequested := slices.Contains(bump, true)
		if !bumpRequested || bumpRequested && !omitReportingPreBump {
			fmt.Println(out)
		}

		if bumpRequested {
			bumpfactor := cctx.Uint64("bump-gas-factor")

			// Recreate the stores.
			overlayBs = blockstore.NewTieredBstore(bs, blockstore.NewMemorySync())
			tmpCs = store.NewChainStore(overlayBs, overlayBs, mds, filcns.Weight, nil)
			tmpCs.StoreEvents(true)

			tmpSm, err = stmgr.NewStateManager(tmpCs, tse, syscalls, upSched, drand)
			if err != nil {
				return fmt.Errorf("failed to setup state manager: %w", err)
			}

			var bumpIdxs []int
			for i, b := range bump {
				if !b {
					continue
				}
				bumpIdxs = append(bumpIdxs, i)
				cid := msgs[i].Cid()
				for _, bmsg := range bmsgs {
					for _, m := range append(bmsg.BlsMessages, bmsg.SecpkMessages...) {
						if cid != m.Cid() {
							continue
						}

						m.VMMessage().GasLimit *= int64(bumpfactor)
					}
				}
			}
			color.Blue("  gas bumps requested for messages at indices: %v", bumpIdxs)

			traces := traces[:0]
			tracer := stmgr.InvocationTracer{Trace: &traces}
			_, postRecRoot, err = tse.ApplyBlocks(ctx, tmpSm, incTs.Height(), incTs.ParentState(), bmsgs, execTs.Height(), r, &tracer, true, baseFee, incTs)
			if err != nil {
				return fmt.Errorf("failed to apply blocks: %w", err)
			}

			_, out, err := compareReceipts(ctx, tmpCs, receipts, postRecRoot, msgs, tracing, traces)
			if err != nil {
				return fmt.Errorf("failed to compare receipts: %w", err)
			}
			fmt.Println(out)
		}

		i = int(execTs.Height())
	}

	return nil
}

func compareReceipts(ctx context.Context, cs *store.ChainStore, oldReceipts []*types.MessageReceipt, newReceiptsRoot cid.Cid, msgs []types.ChainMsg, tracing bool, traces []*api.InvocResult) ([]bool, string, error) {
	var out strings.Builder

	newReceipts, err := loadReceipts(ctx, cs, newReceiptsRoot)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load receipts: %w", err)
	}

	if len(oldReceipts) != len(newReceipts) {
		return nil, "", fmt.Errorf("length of receipts didn't match; %d (orig) != %d (new)", len(oldReceipts), len(newReceipts))
	}

	bump := make([]bool, len(msgs))
	for i, prev := range oldReceipts {
		new_ := newReceipts[i]
		var failures []string
		var dumpTrace bool

		deleteTrace(msgs[i].Cid().String()) //nolint

		if new_.ExitCode != prev.ExitCode {
			failures = append(failures, fmt.Sprintf("EXIT_CODE mismatch: %d (new) != %d (prev)", new_.ExitCode, prev.ExitCode))
			isOOG := new_.ExitCode == exitcode.SysErrOutOfGas
			bump[i] = isOOG
			if !isOOG {
				dumpTrace = true
			}
		}

		if new_.GasUsed != prev.GasUsed {
			failures = append(failures, fmt.Sprintf("GAS_USED mismatch: %d (new) != %d (prev) [%+.4f%%]", new_.GasUsed, prev.GasUsed, (float64(new_.GasUsed-prev.GasUsed)/float64(prev.GasUsed))*100))
		}

		if !slices.Equal(new_.Return, prev.Return) {
			dumpTrace = true
			failures = append(failures, fmt.Sprintf("RETURN mismatch: %s (new) != %s (prev)", hex.EncodeToString(new_.Return), hex.EncodeToString(prev.Return)))
		}

		if !reflect.DeepEqual(new_.EventsRoot, prev.EventsRoot) {
			failures = append(failures, fmt.Sprintf("EVENTS_ROOT mismatch: %s (new) != %s (prev)", new_.EventsRoot, prev.EventsRoot))

			var newEvs, oldEvs []types.Event
			if new_.EventsRoot != nil {
				// Once again, this construction voodoo is not right and is highly coupled with the internals of ChainAPI.
				newEvs, err = (&full.ChainAPI{ExposedBlockstore: cs.StateBlockstore()}).ChainGetEvents(ctx, *new_.EventsRoot)
				if err != nil {
					failures = append(failures, fmt.Sprintf("failed to load new events: %s", err))
					goto Failures
				}
			}
			if prev.EventsRoot != nil {
				oldEvs, err = (&full.ChainAPI{ExposedBlockstore: cs.StateBlockstore()}).ChainGetEvents(ctx, *prev.EventsRoot)
				if err != nil {
					failures = append(failures, fmt.Sprintf("failed to load old events: %s", err))
					goto Failures
				}
			}

			if !reflect.DeepEqual(newEvs, oldEvs) {
				dumpTrace = true
				failures[len(failures)-1] += "; (!) events ARE NOT not semantically equivalent"
			} else {
				failures[len(failures)-1] += "; but events ARE semantically equivalent"
			}
		}

	Failures:
		if len(failures) == 0 {
			out.WriteString(color.GreenString("  [%d] (%s): RECEIPTS EQUAL", i, msgs[i].Cid()) + "\n")
		} else {
			out.WriteString(color.RedString("  [%d] (%s): RECEIPTS MISMATCH", i, msgs[i].Cid()) + "\n")
			for _, f := range failures {
				out.WriteString(color.RedString("    - "+f) + "\n")
			}
		}

		if tracing && dumpTrace {
			if err := writeTrace(msgs[i].Cid().String(), traces[i]); err != nil {
				out.WriteString(color.RedString("  failed to write trace: %s", err))
			}
		}
	}

	return bump, out.String(), nil
}

func writeTrace(filename string, trace *api.InvocResult) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close() //nolint
	if err := json.NewEncoder(file).Encode(trace); err != nil {
		return fmt.Errorf("failed to encode trace: %w", err)
	}
	return nil
}

func deleteTrace(filename string) error {
	return os.Remove(filename)
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
	_ = a.ForEach(&r, func(i int64) error {
		ret = append(ret, r)
		return nil
	})
	return ret, nil
}
