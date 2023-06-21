package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var mismatchesCmd = &cli.Command{
	Name:        "mismatches",
	Description: "Walk up the chain, recomputing state, and reporting any mismatches",
	Action: func(cctx *cli.Context) error {
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		checkTs, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		for checkTs.Height() != 0 {
			if checkTs.Height()%10000 == 0 {
				fmt.Println("Reached height ", checkTs.Height())
			}

			execTsk := checkTs.Parents()
			execTs, err := api.ChainGetTipSet(ctx, execTsk)
			if err != nil {
				return err
			}

			st, err := api.StateCompute(ctx, execTs.Height(), nil, execTsk)
			if err != nil {
				return err
			}

			if st.Root != checkTs.ParentState() {
				fmt.Println("consensus mismatch found at height ", execTs.Height())
			}

			checkTs = execTs
		}

		return nil
	},
}
