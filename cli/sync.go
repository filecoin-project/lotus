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
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
)

var SyncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Inspect or interact with the chain syncer",
	Subcommands: []*cli.Command{
		SyncStatusCmd,
		SyncWaitCmd,
		SyncMarkBadCmd,
		SyncUnmarkBadCmd,
		SyncCheckBadCmd,
		SyncCheckpointCmd,
	},
}

var SyncStatusCmd = &cli.Command{
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
		for _, ss := range state.ActiveSyncs {
			fmt.Printf("worker %d:\n", ss.WorkerID)
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

var SyncWaitCmd = &cli.Command{
	Name:  "wait",
	Usage: "Wait for sync to be complete",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "don't exit after node is synced",
		},
	},
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		return SyncWait(ctx, napi, cctx.Bool("watch"))
	},
}

var SyncMarkBadCmd = &cli.Command{
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

var SyncUnmarkBadCmd = &cli.Command{
	Name:  "unmark-bad",
	Usage: "Unmark the given block as bad, makes it possible to sync to a chain containing it",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "drop the entire bad block cache",
		},
	},
	ArgsUsage: "[blockCid]",
	Action: func(cctx *cli.Context) error {
		napi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Bool("all") {
			return napi.SyncUnmarkAllBad(ctx)
		}

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

var SyncCheckBadCmd = &cli.Command{
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

var SyncCheckpointCmd = &cli.Command{
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

func SyncWait(ctx context.Context, napi v0api.FullNode, watch bool) error {
	tick := time.Second / 4

	lastLines := 0
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	samples := 8
	i := 0
	var firstApp, app, lastApp uint64

	state, err := napi.SyncState(ctx)
	if err != nil {
		return err
	}
	firstApp = state.VMApplied

	for {
		state, err := napi.SyncState(ctx)
		if err != nil {
			return err
		}

		if len(state.ActiveSyncs) == 0 {
			time.Sleep(time.Second)
			continue
		}

		head, err := napi.ChainHead(ctx)
		if err != nil {
			return err
		}

		working := -1
		for i, ss := range state.ActiveSyncs {
			switch ss.Stage {
			case api.StageSyncComplete:
			default:
				working = i
			case api.StageIdle:
				// not complete, not actively working
			}
		}

		if working == -1 {
			working = len(state.ActiveSyncs) - 1
		}

		ss := state.ActiveSyncs[working]
		workerID := ss.WorkerID

		var baseHeight abi.ChainEpoch
		var target []cid.Cid
		var theight abi.ChainEpoch
		var heightDiff int64

		if ss.Base != nil {
			baseHeight = ss.Base.Height()
			heightDiff = int64(ss.Base.Height())
		}
		if ss.Target != nil {
			target = ss.Target.Cids()
			theight = ss.Target.Height()
			heightDiff = int64(ss.Target.Height()) - heightDiff
		} else {
			heightDiff = 0
		}

		for i := 0; i < lastLines; i++ {
			fmt.Print("\r\x1b[2K\x1b[A")
		}

		fmt.Printf("Worker: %d; Base: %d; Target: %d (diff: %d)\n", workerID, baseHeight, theight, heightDiff)
		fmt.Printf("State: %s; Current Epoch: %d; Todo: %d\n", ss.Stage, ss.Height, theight-ss.Height)
		lastLines = 2

		if i%samples == 0 {
			lastApp = app
			app = state.VMApplied - firstApp
		}
		if i > 0 {
			fmt.Printf("Validated %d messages (%d per second)\n", state.VMApplied-firstApp, (app-lastApp)*uint64(time.Second/tick)/uint64(samples))
			lastLines++
		}

		_ = target // todo: maybe print? (creates a bunch of line wrapping issues with most tipsets)

		if !watch && time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs) {
			fmt.Println("\nDone!")
			return nil
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nExit by user")
			return nil
		case <-ticker.C:
		}

		i++
	}
}
