package main

import (
	"fmt"
	"strconv"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
)

var storeGarbageCmd = &cli.Command{
	Name:  "store-garbage",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		return nodeApi.StoreGarbageData(ctx)
	},
}

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		sectorsStatusCmd,
		sectorsListCmd,
		sectorsRefsCmd,
	},
}

var sectorsStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "Get the seal status of a sector by its ID",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify sector ID to get status of")
		}

		id, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		status, err := nodeApi.SectorsStatus(ctx, id)
		if err != nil {
			return err
		}

		fmt.Printf("SectorID:\t%d\n", status.SectorID)
		fmt.Printf("Status:\t%s\n", api.SectorStateStr(status.State))
		fmt.Printf("CommD:\t\t%x\n", status.CommD)
		fmt.Printf("CommR:\t\t%x\n", status.CommR)
		fmt.Printf("Ticket:\t\t%x\n", status.Ticket.TicketBytes)
		fmt.Printf("TicketH:\t\t%d\n", status.Ticket.BlockHeight)
		fmt.Printf("Seed:\t\t%x\n", status.Seed.TicketBytes)
		fmt.Printf("SeedH:\t\t%d\n", status.Seed.BlockHeight)
		fmt.Printf("Proof:\t\t%x\n", status.Proof)
		fmt.Printf("Deals:\t\t%v\n", status.Deals)
		return nil
	},
}

var sectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		fullApi, closer2, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		list, err := nodeApi.SectorsList(ctx)
		if err != nil {
			return err
		}

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		pset, err := fullApi.StateMinerProvingSet(ctx, maddr, nil)
		if err != nil {
			return err
		}
		provingIDs := make(map[uint64]struct{}, len(pset))
		for _, info := range pset {
			provingIDs[info.SectorID] = struct{}{}
		}

		sset, err := fullApi.StateMinerSectors(ctx, maddr, nil)
		if err != nil {
			return err
		}
		commitedIDs := make(map[uint64]struct{}, len(pset))
		for _, info := range sset {
			commitedIDs[info.SectorID] = struct{}{}
		}

		for _, s := range list {
			st, err := nodeApi.SectorsStatus(ctx, s)
			if err != nil {
				fmt.Printf("%d:\tError: %s\n", s, err)
				continue
			}

			_, inSSet := commitedIDs[s]
			_, inPSet := provingIDs[s]

			fmt.Printf("%d: %s\tsSet: %s\tpSet: %s\ttktH: %d\tseedH: %d\tdeals: %v\n",
				s,
				api.SectorStateStr(st.State),
				yesno(inSSet),
				yesno(inPSet),
				st.Ticket.BlockHeight,
				st.Seed.BlockHeight,
				st.Deals,
			)
		}
		return nil
	},
}

var sectorsRefsCmd = &cli.Command{
	Name:  "refs",
	Usage: "List References to sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		refs, err := nodeApi.SectorsRefs(ctx)
		if err != nil {
			return err
		}

		for name, refs := range refs {
			fmt.Printf("Block %s:\n", name)
			for _, ref := range refs {
				fmt.Printf("\t%s+%d %d bytes\n", ref.Piece, ref.Offset, ref.Size)
			}
		}
		return nil
	},
}

func yesno(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}
