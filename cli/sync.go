package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/chain"
	cid "github.com/ipfs/go-cid"
)

var syncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Inspect or interact with the chain syncer",
	Subcommands: []*cli.Command{
		syncStatusCmd,
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
