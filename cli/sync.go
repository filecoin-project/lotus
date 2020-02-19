package cli

import (
	"context"
	"fmt"
	"time"

	cid "github.com/ipfs/go-cid"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
)

var syncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Inspect or interact with the chain syncer",
	Subcommands: []*cli.Command{
		syncStatusCmd,
		syncWaitCmd,
		syncMarkBadCmd,
		syncCheckBadCmd,
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
			var theight uint64
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
			fmt.Printf("\tStage: %s\n", chain.SyncStageString(ss.Stage))
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
	Name:  "mark-bad",
	Usage: "Mark the given block as bad, will prevent syncing to a chain that contains it",
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

var syncCheckBadCmd = &cli.Command{
	Name:  "check-bad",
	Usage: "check if the given block was marked bad, and for what reason",
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
		if ss.Target != nil {
			target = ss.Target.Cids()
		}

		fmt.Printf("\r\x1b[2KWorker %d: Target: %s\tState: %s\tHeight: %d", working, target, chain.SyncStageString(ss.Stage), ss.Height)

		if time.Now().Unix()-int64(head.MinTimestamp()) < build.BlockDelay {
			fmt.Println("\nDone!")
			return nil
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nExit by user")
			return nil
		case <-time.After(1 * time.Second):
		}
	}
}
