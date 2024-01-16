package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	verifierffi "github.com/filecoin-project/lotus/chain/verifier/ffi"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

const MAINNET_GENESIS_TIME = 1598306400

// USAGE: Sync a node, then call migrate-nv17 on some old state. Pass in the cid of the migrated state root,
// the epoch you migrated at, the network version you migrated to, and a message CID. You will be able to replay any
// message from between the migration epoch, and where your node originally synced to. Note: You may run into issues
// with state that changed between the epoch you migrated at, and when the message was originally processed.
// This can be avoided by replaying messages from close to the migration epoch, or circumvented by using a custom
// FVM bundle.
var gasTraceCmd = &cli.Command{
	Name:        "trace-gas",
	Description: "replay a message on the specified stateRoot and network version to get an execution trace",
	ArgsUsage:   "[migratedStateRootCid networkVersion messageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 3 {
			return lcli.IncorrectNumArgs(cctx)
		}

		stateRootCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		nv, err := strconv.ParseInt(cctx.Args().Get(1), 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		messageCid, err := cid.Decode(cctx.Args().Get(2))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
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

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), MAINNET_GENESIS_TIME, nil)
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(verifierffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), shd, mds, index.DummyMsgIndex)
		if err != nil {
			return err
		}

		msg, err := cs.GetMessage(ctx, messageCid)
		if err != nil {
			return err
		}

		// Set to block limit so message will not run out of gas
		msg.GasLimit = buildconstants.BlockGasLimit

		err = cs.Load(ctx)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 8, 2, 2, ' ', tabwriter.AlignRight)
		res, err := sm.CallAtStateAndVersion(ctx, msg, stateRootCid, network.Version(nv))
		if err != nil {
			return err
		}
		fmt.Println("Total gas used: ", res.MsgRct.GasUsed)
		printInternalExecutions(0, []types.ExecutionTrace{res.ExecutionTrace}, tw)

		return tw.Flush()
	},
}

var replayOfflineCmd = &cli.Command{
	Name:        "replay-offline",
	Description: "replay a message to get a gas trace",
	ArgsUsage:   "[messageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.Int64Flag{
			Name:  "lookback-limit",
			Value: 10000,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		messageCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		lookbackLimit := cctx.Int("lookback-limit")

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
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

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), MAINNET_GENESIS_TIME, nil)
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(verifierffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), shd, mds, index.DummyMsgIndex)
		if err != nil {
			return err
		}

		msg, err := cs.GetMessage(ctx, messageCid)
		if err != nil {
			return err
		}

		err = cs.Load(ctx)
		if err != nil {
			return err
		}

		ts, _, _, err := sm.SearchForMessage(ctx, cs.GetHeaviestTipSet(), messageCid, abi.ChainEpoch(lookbackLimit), true)
		if err != nil {
			return err
		}
		if ts == nil {
			return xerrors.Errorf("could not find message within the last %d epochs", lookbackLimit)
		}
		executionTs, err := cs.GetTipsetByHeight(ctx, ts.Height()-2, ts, true)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 8, 2, 2, ' ', tabwriter.AlignRight)
		res, err := sm.CallWithGas(ctx, msg, []types.ChainMsg{}, executionTs, true)
		if err != nil {
			return err
		}
		fmt.Println("Total gas used: ", res.MsgRct.GasUsed)
		printInternalExecutions(0, []types.ExecutionTrace{res.ExecutionTrace}, tw)

		return tw.Flush()
	},
}

func printInternalExecutions(depth int, trace []types.ExecutionTrace, tw *tabwriter.Writer) {
	if depth == 0 {
		_, _ = fmt.Fprintf(tw, "Depth\tFrom\tTo\tMethod\tTotalGas\tComputeGas\tStorageGas\t\tExitCode\n")
	}
	for _, im := range trace {
		sumGas := im.SumGas()
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%d\t%d\t%d\t\t%d\n", depth, truncateString(im.Msg.From.String(), 10), truncateString(im.Msg.To.String(), 10), im.Msg.Method, sumGas.TotalGas, sumGas.ComputeGas, sumGas.StorageGas, im.MsgRct.ExitCode)
		printInternalExecutions(depth+1, im.Subcalls, tw)
	}
}

func truncateString(str string, length int) string {
	if len(str) <= length {
		return str
	}

	truncated := ""
	count := 0
	for _, char := range str {
		truncated += string(char)
		count++
		if count >= length {
			break
		}
	}
	truncated += "..."
	return truncated
}
