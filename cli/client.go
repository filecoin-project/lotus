package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	lapi "github.com/filecoin-project/lotus/api"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

var clientCmd = &cli.Command{
	Name:  "client",
	Usage: "Make deals, store data, retrieve data",
	Subcommands: []*cli.Command{
		clientImportCmd,
		clientLocalCmd,
		clientDealCmd,
		clientFindCmd,
		clientRetrieveCmd,
		clientQueryAskCmd,
		clientListDeals,
	},
}

var clientImportCmd = &cli.Command{
	Name:  "import",
	Usage: "Import data",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		absPath, err := filepath.Abs(cctx.Args().First())
		if err != nil {
			return err
		}

		c, err := api.ClientImport(ctx, absPath)
		if err != nil {
			return err
		}
		fmt.Println(c.String())
		return nil
	},
}

var clientLocalCmd = &cli.Command{
	Name:  "local",
	Usage: "List locally imported data",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		list, err := api.ClientListImports(ctx)
		if err != nil {
			return err
		}
		for _, v := range list {
			fmt.Printf("%s %s %d %s\n", v.Key, v.FilePath, v.Size, v.Status)
		}
		return nil
	},
}

var clientDealCmd = &cli.Command{
	Name:  "deal",
	Usage: "Initialize storage deal with a miner",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.NArg() != 4 {
			return xerrors.New("expected 4 args: dataCid, miner, price, duration")
		}

		// [data, miner, dur]

		data, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		miner, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		price, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		dur, err := strconv.ParseInt(cctx.Args().Get(3), 10, 32)
		if err != nil {
			return err
		}

		a, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}
		proposal, err := api.ClientStartDeal(ctx, data, a, miner, types.BigInt(price), uint64(dur))
		if err != nil {
			return err
		}

		fmt.Println(proposal)
		return nil
	},
}

var clientFindCmd = &cli.Command{
	Name:  "find",
	Usage: "find data in the network",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			fmt.Println("Usage: find [CID]")
			return nil
		}

		file, err := cid.Parse(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		// Check if we already have this data locally

		has, err := api.ClientHasLocal(ctx, file)
		if err != nil {
			return err
		}

		if has {
			fmt.Println("LOCAL")
		}

		offers, err := api.ClientFindData(ctx, file)
		if err != nil {
			return err
		}

		for _, offer := range offers {
			if offer.Err != "" {
				fmt.Printf("ERR %s@%s: %s\n", offer.Miner, offer.MinerPeerID, offer.Err)
				continue
			}
			fmt.Printf("RETRIEVAL %s@%s-%sfil-%db\n", offer.Miner, offer.MinerPeerID, types.FIL(offer.MinPrice), offer.Size)
		}

		return nil
	},
}

var clientRetrieveCmd = &cli.Command{
	Name:  "retrieve",
	Usage: "retrieve data from network",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "address",
			Usage: "address to use for transactions",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			fmt.Println("Usage: retrieve [CID] [outfile]")
			return nil
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var payer address.Address
		if cctx.String("address") != "" {
			payer, err = address.NewFromString(cctx.String("address"))
		} else {
			payer, err = api.WalletDefaultAddress(ctx)
		}
		if err != nil {
			return err
		}

		file, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		// Check if we already have this data locally

		/*has, err := api.ClientHasLocal(ctx, file)
		if err != nil {
			return err
		}

		if has {
			fmt.Println("Success: Already in local storage")
			return nil
		}*/ // TODO: fix

		offers, err := api.ClientFindData(ctx, file)
		if err != nil {
			return err
		}

		// TODO: parse offer strings from `client find`, make this smarter

		if len(offers) < 1 {
			fmt.Println("Failed to find file")
			return nil
		}

		if err := api.ClientRetrieve(ctx, offers[0].Order(payer), cctx.Args().Get(1)); err != nil {
			return xerrors.Errorf("Retrieval Failed: %w", err)
		}

		fmt.Println("Success")
		return nil
	},
}

var clientQueryAskCmd = &cli.Command{
	Name:  "query-ask",
	Usage: "find a miners ask",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "peerid",
			Usage: "specify peer ID of node to make query against",
		},
		&cli.Int64Flag{
			Name:  "size",
			Usage: "data size in bytes",
		},
		&cli.Int64Flag{
			Name:  "duration",
			Usage: "deal duration",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			fmt.Println("Usage: query-ask [address]")
			return nil
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var pid peer.ID
		if pidstr := cctx.String("peerid"); pidstr != "" {
			p, err := peer.IDFromString(pidstr)
			if err != nil {
				return err
			}
			pid = p
		} else {
			ret, err := api.StateCall(ctx, &types.Message{
				To:     maddr,
				From:   maddr,
				Method: actors.MAMethods.GetPeerID,
			}, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("failed to get peerID for miner: %w", err)
			}

			if ret.ExitCode != 0 {
				return fmt.Errorf("call to GetPeerID was unsuccesful (exit code %d)", ret.ExitCode)
			}
			if peer.ID(ret.Return) == peer.ID("SETME") {
				return fmt.Errorf("the miner hasn't initialized yet")
			}

			p, err := peer.IDFromBytes(ret.Return)
			if err != nil {
				return err
			}

			pid = p
		}

		ask, err := api.ClientQueryAsk(ctx, pid, maddr)
		if err != nil {
			return err
		}

		fmt.Printf("Ask: %s\n", maddr)
		fmt.Printf("Price per GiB: %s\n", types.FIL(ask.Ask.Price))

		size := cctx.Int64("size")
		if size == 0 {
			return nil
		}
		perEpoch := types.BigDiv(types.BigMul(ask.Ask.Price, types.NewInt(uint64(size))), types.NewInt(1<<30))
		fmt.Printf("Price per Block: %s\n", types.FIL(perEpoch))

		duration := cctx.Int64("duration")
		if duration == 0 {
			return nil
		}
		fmt.Printf("Total Price: %s\n", types.FIL(types.BigMul(perEpoch, types.NewInt(uint64(duration)))))

		return nil
	},
}

var clientListDeals = &cli.Command{
	Name:  "list-deals",
	Usage: "List storage market deals",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		deals, err := api.ClientListDeals(ctx)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintf(w, "DealCid\tProvider\tState\tPieceRef\tSize\tPrice\tDuration\n")
		for _, d := range deals {
			fmt.Fprintf(w, "%s\t%s\t%s\t%x\t%d\t%s\t%d\n", d.ProposalCid, d.Provider, lapi.DealStates[d.State], d.PieceRef, d.Size, d.PricePerEpoch, d.Duration)
		}
		return w.Flush()
	},
}
