package cli

import (
	"fmt"
	"time"

	cid "github.com/ipfs/go-cid"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
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

		ss, err := api.SyncState(ctx)
		if err != nil {
			return err
		}

		var base, target []cid.Cid
		if ss.Base != nil {
			base = ss.Base.Cids()
		}
		if ss.Target != nil {
			target = ss.Target.Cids()
		}

		fmt.Println("sync status:")
		fmt.Printf("Base:\t%s\n", base)
		fmt.Printf("Target:\t%s\n", target)
		fmt.Printf("Stage: %s\n", chain.SyncStageString(ss.Stage))
		fmt.Printf("Height: %d\n", ss.Height)
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
			ss, err := napi.SyncState(ctx)
			if err != nil {
				return err
			}

			var target []cid.Cid
			if ss.Target != nil {
				target = ss.Target.Cids()
			}

			fmt.Printf("\r\x1b[2KTarget: %s\tState: %s\tHeight: %d", target, chain.SyncStageString(ss.Stage), ss.Height)
			if ss.Stage == api.StageSyncComplete {
				fmt.Println("\nDone")
				return nil
			}

			time.Sleep(1 * time.Second)
		}
	},
}
