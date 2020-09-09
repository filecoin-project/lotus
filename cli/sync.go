package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-state-types/abi"
	cid "github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
)

var syncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Inspect or interact with the chain syncer",
	Subcommands: []*cli.Command{
		syncStatusCmd,
		syncWaitCmd,
		syncMarkBadCmd,
		syncUnmarkBadCmd,
		syncCheckBadCmd,
		syncCheckpointCmd,
	},
}

var syncStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "check sync status",
	Action: func(cctx *cli.Context) error {
		apic, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		state, err := apic.SyncState(ctx)
		if err != nil {
			return err
		}

		fmt.Println("sync status:")
		for i, ss := range state.ActiveSyncs {
			fmt.Printf("worker %d:\n", i)
			var base, target []cid.Cid
			var heightDiff int64
			var theight abi.ChainEpoch
			if ss.Base != nil {
				base = ss.Base.Cids()
				heightDiff = int64(ss.Base.Height())
			}
			if ss.Target != nil {
				target = ss.Target.Cids()
				heightDiff = int64(ss.Target.Height()) - heightDiff
				theight = ss.Target.Height()
			} else {
				heightDiff = 0
			}
			fmt.Printf("\tBase:\t%s\n", base)
			fmt.Printf("\tTarget:\t%s (%d)\n", target, theight)
			fmt.Printf("\tHeight diff:\t%d\n", heightDiff)
			fmt.Printf("\tStage: %s\n", ss.Stage)
			fmt.Printf("\tHeight: %d\n", ss.Height)
			if ss.End.IsZero() {
				if !ss.Start.IsZero() {
					fmt.Printf("\tElapsed: %s\n", time.Since(ss.Start))
				}
			} else {
				fmt.Printf("\tElapsed: %s\n", ss.End.Sub(ss.Start))
			}
			if ss.Stage == api.StageSyncErrored {
				fmt.Printf("\tError: %s\n", ss.Message)
			}
		}
		return nil
	},
}

var syncWaitCmd = &cli.Command{
	Name:  "wait",
	Usage: "Wait for sync to be complete",
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return SyncWait(ctx, napi)
	},
}

var syncMarkBadCmd = &cli.Command{
	Name:      "mark-bad",
	Usage:     "Mark the given block as bad, will prevent syncing to a chain that contains it",
	ArgsUsage: "[blockCid]",
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify block cid to mark")
		}

		bcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to decode input as a cid: %s", err)
		}

		return napi.SyncMarkBad(ctx, bcid)
	},
}

var syncUnmarkBadCmd = &cli.Command{
	Name:      "unmark-bad",
	Usage:     "Unmark the given block as bad, makes it possible to sync to a chain containing it",
	ArgsUsage: "[blockCid]",
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify block cid to unmark")
		}

		bcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to decode input as a cid: %s", err)
		}

		return napi.SyncUnmarkBad(ctx, bcid)
	},
}

var syncCheckBadCmd = &cli.Command{
	Name:      "check-bad",
	Usage:     "check if the given block was marked bad, and for what reason",
	ArgsUsage: "[blockCid]",
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify block cid to check")
		}

		bcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to decode input as a cid: %s", err)
		}

		reason, err := napi.SyncCheckBad(ctx, bcid)
		if err != nil {
			return err
		}

		if reason == "" {
			fmt.Println("block was not marked as bad")
			return nil
		}

		fmt.Println(reason)
		return nil
	},
}

var syncCheckpointCmd = &cli.Command{
	Name:      "checkpoint",
	Usage:     "mark a certain tipset as checkpointed; the node will never fork away from this tipset",
	ArgsUsage: "[tipsetKey]",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "epoch",
			Usage: "checkpoint the tipset at the given epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var ts *types.TipSet

		if cctx.IsSet("epoch") {
			ts, err = napi.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Uint64("epoch")), types.EmptyTSK)
		}
		if ts == nil {
			ts, err = parseTipSet(ctx, napi, cctx.Args().Slice())
		}
		if err != nil {
			return err
		}

		if ts == nil {
			return fmt.Errorf("must pass cids for tipset to set as head, or specify epoch flag")
		}

		if err := napi.SyncCheckpoint(ctx, ts.Key()); err != nil {
			return err
		}

		return nil
	},
}

func SyncWait(ctx context.Context, napi api.FullNode) error {
	for {
		state, err := napi.SyncState(ctx)
		if err != nil {
			return err
		}

		head, err := napi.ChainHead(ctx)
		if err != nil {
			return err
		}

		working := 0
		for i, ss := range state.ActiveSyncs {
			switch ss.Stage {
			case api.StageSyncComplete:
			default:
				working = i
			case api.StageIdle:
				// not complete, not actively working
			}
		}

		ss := state.ActiveSyncs[working]

		var target []cid.Cid
		var theight abi.ChainEpoch
		if ss.Target != nil {
			target = ss.Target.Cids()
			theight = ss.Target.Height()
		}

		fmt.Printf("\r\x1b[2KWorker %d: Target Height: %d\tTarget: %s\tState: %s\tHeight: %d", working, theight, target, ss.Stage, ss.Height)

		if time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs) {
			fmt.Println("\nDone!")
			return nil
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nExit by user")
			return nil
		case <-build.Clock.After(1 * time.Second):
		}
	}
}
