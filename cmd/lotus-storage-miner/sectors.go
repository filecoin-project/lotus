package main

import (
	"fmt"
	"github.com/filecoin-project/lotus/chain/types"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
)

var pledgeSectorCmd = &cli.Command{
	Name:  "pledge-sector",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		return nodeApi.PledgeSector(ctx)
	},
}

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		sectorsStatusCmd,
		sectorsListCmd,
		sectorsRefsCmd,
		sectorsUpdateCmd,
	},
}

var sectorsStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "Get the seal status of a sector by its ID",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "log",
			Usage: "display event log",
		},
	},
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
		fmt.Printf("Status:\t%s\n", api.SectorStates[status.State])
		fmt.Printf("CommD:\t\t%x\n", status.CommD)
		fmt.Printf("CommR:\t\t%x\n", status.CommR)
		fmt.Printf("Ticket:\t\t%x\n", status.Ticket.TicketBytes)
		fmt.Printf("TicketH:\t\t%d\n", status.Ticket.BlockHeight)
		fmt.Printf("Seed:\t\t%x\n", status.Seed.TicketBytes)
		fmt.Printf("SeedH:\t\t%d\n", status.Seed.BlockHeight)
		fmt.Printf("Proof:\t\t%x\n", status.Proof)
		fmt.Printf("Deals:\t\t%v\n", status.Deals)
		fmt.Printf("Retries:\t\t%d\n", status.Retries)
		if status.LastErr != "" {
			fmt.Printf("Last Error:\t\t%s\n", status.LastErr)
		}

		if cctx.Bool("log") {
			fmt.Printf("--------\nEvent Log:\n")

			for i, l := range status.Log {
				fmt.Printf("%d.\t%s:\t[%s]\t%s\n", i, time.Unix(int64(l.Timestamp), 0), l.Kind, l.Message)
				if l.Trace != "" {
					fmt.Printf("\t%s\n", l.Trace)
				}
			}
		}
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

		pset, err := fullApi.StateMinerProvingSet(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		provingIDs := make(map[uint64]struct{}, len(pset))
		for _, info := range pset {
			provingIDs[info.SectorID] = struct{}{}
		}

		sset, err := fullApi.StateMinerSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		commitedIDs := make(map[uint64]struct{}, len(pset))
		for _, info := range sset {
			commitedIDs[info.SectorID] = struct{}{}
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i] < list[j]
		})

		w := tabwriter.NewWriter(os.Stdout, 8, 4, 1, ' ', 0)

		for _, s := range list {
			st, err := nodeApi.SectorsStatus(ctx, s)
			if err != nil {
				fmt.Fprintf(w, "%d:\tError: %s\n", s, err)
				continue
			}

			_, inSSet := commitedIDs[s]
			_, inPSet := provingIDs[s]

			fmt.Fprintf(w, "%d: %s\tsSet: %s\tpSet: %s\ttktH: %d\tseedH: %d\tdeals: %v\n",
				s,
				api.SectorStates[st.State],
				yesno(inSSet),
				yesno(inPSet),
				st.Ticket.BlockHeight,
				st.Seed.BlockHeight,
				st.Deals,
			)
		}

		return w.Flush()
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
				fmt.Printf("\t%d+%d %d bytes\n", ref.SectorID, ref.Offset, ref.Size)
			}
		}
		return nil
	},
}

var sectorsUpdateCmd = &cli.Command{
	Name:  "update-state",
	Usage: "ADVANCED: manually update the state of a sector, this may aid in error recovery",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("this is a command for advanced users, only use it if you are sure of what you are doing")
		}
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() < 2 {
			return xerrors.Errorf("must pass sector ID and new state")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector ID: %w", err)
		}

		var st api.SectorState
		for i, s := range api.SectorStates {
			if cctx.Args().Get(1) == s {
				st = api.SectorState(i)
			}
		}

		return nodeApi.SectorsUpdate(ctx, id, st)
	},
}

func yesno(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}
