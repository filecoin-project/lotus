package cli

import (
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
	},
}

var syncStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "check sync status",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		state, err := api.SyncState(ctx)
		if err != nil {
			return err
		}

		fmt.Println("sync status:")
		for i, ss := range state.ActiveSyncs {
			fmt.Printf("worker %d:\n", i)
			var base, target []cid.Cid
			var heightDiff int64
			if ss.Base != nil {
				base = ss.Base.Cids()
				heightDiff = int64(ss.Base.Height())
			}
			if ss.Target != nil {
				target = ss.Target.Cids()
				heightDiff = int64(ss.Target.Height()) - heightDiff
			} else {
				heightDiff = 0
			}
			fmt.Printf("\tBase:\t%s\n", base)
			fmt.Printf("\tTarget:\t%s\n", target)
			fmt.Printf("\tHeight diff:\t%d\n", heightDiff)
			fmt.Printf("\tStage: %s\n", chain.SyncStageString(ss.Stage))
			fmt.Printf("\tHeight: %d\n", ss.Height)
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
	},
}
