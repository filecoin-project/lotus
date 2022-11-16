package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
)

var gasEstimationCmd = &cli.Command{
	Name:        "estimate-gas",
	Description: "replay a message on the specified stateRoot and network version",
	ArgsUsage:   "[migratedStateRootCid migrationEpoch networkVersion messageHash]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 4 {
			return lcli.IncorrectNumArgs(cctx)
		}

		stateRootCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		epoch, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		nv, err := strconv.ParseInt(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		messageCid, err := cid.Decode(cctx.Args().Get(3))
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

		dcs := build.DrandConfigSchedule()
		shd := beacon.Schedule{}
		for _, dc := range dcs {
			bc, err := drand.NewDrandBeacon(1598306400, build.BlockDelaySecs, nil, dc.Config)
			if err != nil {
				return xerrors.Errorf("creating drand beacon: %w", err)
			}
			shd = append(shd, beacon.BeaconPoint{Start: dc.Start, Beacon: bc})
		}
		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		sm, err := stmgr.NewStateManager(cs, filcns.NewTipSetExecutor(), vm.Syscalls(ffiwrapper.ProofVerifier), filcns.DefaultUpgradeSchedule(), shd)
		if err != nil {
			return err
		}

		msg, err := cs.GetMessage(ctx, messageCid)
		if err != nil {
			return err
		}

		// Set to block limit so message will not run out of gas
		msg.GasLimit = 10_000_000_000

		err = cs.Load(ctx)
		if err != nil {
			return err
		}

		executionTs, err := cs.GetTipsetByHeight(ctx, abi.ChainEpoch(epoch), nil, false)
		if err != nil {
			return err
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 2, 2, ' ', 0)
		res, err := sm.CallAtStateAndVersion(ctx, msg, executionTs, stateRootCid, network.Version(nv))
		if err != nil {
			return err
		}
		printInternalExecutions(0, []types.ExecutionTrace{res.ExecutionTrace}, tw)

		return tw.Flush()
	},
}

func printInternalExecutions(depth int, trace []types.ExecutionTrace, tw *tabwriter.Writer) {
	if depth == 0 {
		_, _ = fmt.Fprintf(tw, "depth\tFrom\tTo\tValue\tMethod\tGasUsed\tExitCode\tReturn\n")
	}
	for _, im := range trace {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%d\t%d\t%x\n", depth, im.Msg.From, im.Msg.To, im.Msg.Value, im.Msg.Method, im.MsgRct.GasUsed, im.MsgRct.ExitCode, im.MsgRct.Return)
		printInternalExecutions(depth+1, im.Subcalls, tw)
	}
}
