package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var piecesCmd = &cli.Command{
	Name:        "pieces",
	Usage:       "interact with the piecestore",
	Description: "The piecestore is a database that tracks and manages data that is made available to the retrieval market",
	Subcommands: []*cli.Command{
		piecesListPiecesCmd,
		piecesListCidInfosCmd,
		piecesInfoCmd,
		piecesCidInfoCmd,
	},
}

var piecesListPiecesCmd = &cli.Command{
	Name:  "list-pieces",
	Usage: "list registered pieces",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		pieceCids, err := nodeApi.PiecesListPieces(ctx)
		if err != nil {
			return err
		}

		for _, pc := range pieceCids {
			fmt.Println(pc)
		}
		return nil
	},
}

var piecesListCidInfosCmd = &cli.Command{
	Name:  "list-cids",
	Usage: "list registered payload CIDs",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		cids, err := nodeApi.PiecesListCidInfos(ctx)
		if err != nil {
			return err
		}

		for _, c := range cids {
			fmt.Println(c)
		}
		return nil
	},
}

var piecesInfoCmd = &cli.Command{
	Name:  "piece-info",
	Usage: "get registered information for a given piece CID",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify piece cid"))
		}

		nodeApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		pi, err := nodeApi.PiecesGetPieceInfo(ctx, c)
		if err != nil {
			return err
		}

		fmt.Println("Piece: ", pi.PieceCID)
		w := tabwriter.NewWriter(os.Stdout, 4, 4, 2, ' ', 0)
		fmt.Fprintln(w, "Deals:\nDealID\tSectorID\tLength\tOffset")
		for _, d := range pi.Deals {
			fmt.Fprintf(w, "%d\t%d\t%d\t%d\n", d.DealID, d.SectorID, d.Length, d.Offset)
		}
		return w.Flush()
	},
}

var piecesCidInfoCmd = &cli.Command{
	Name:  "cid-info",
	Usage: "get registered information for a given payload CID",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify payload cid"))
		}

		nodeApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		ci, err := nodeApi.PiecesGetCIDInfo(ctx, c)
		if err != nil {
			return err
		}

		fmt.Println("Info for: ", ci.CID)

		w := tabwriter.NewWriter(os.Stdout, 4, 4, 2, ' ', 0)
		fmt.Fprintf(w, "PieceCid\tOffset\tSize\n")
		for _, loc := range ci.PieceBlockLocations {
			fmt.Fprintf(w, "%s\t%d\t%d\n", loc.PieceCID, loc.RelOffset, loc.BlockSize)
		}
		return w.Flush()
	},
}
