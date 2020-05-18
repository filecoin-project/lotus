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
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

var clientCmd = &cli.Command{
	Name:  "client",
	Usage: "Make deals, store data, retrieve data",
	Subcommands: []*cli.Command{
		clientImportCmd,
		clientCommPCmd,
		clientLocalCmd,
		clientDealCmd,
		clientFindCmd,
		clientRetrieveCmd,
		clientQueryAskCmd,
		clientListDeals,
		clientCarGenCmd,
	},
}

var clientImportCmd = &cli.Command{
	Name:      "import",
	Usage:     "Import data",
	ArgsUsage: "[inputPath]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "car",
			Usage: "import from a car file instead of a regular file",
		},
	},
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

		ref := lapi.FileRef{
			Path:  absPath,
			IsCAR: cctx.Bool("car"),
		}
		c, err := api.ClientImport(ctx, ref)
		if err != nil {
			return err
		}
		fmt.Println(c.String())
		return nil
	},
}

var clientCommPCmd = &cli.Command{
	Name:      "commP",
	Usage:     "calculate the piece-cid (commP) of a CAR file",
	ArgsUsage: "[inputFile minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("usage: commP <inputPath> <minerAddr>")
		}

		miner, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		ret, err := api.ClientCalcCommP(ctx, cctx.Args().Get(0), miner)

		if err != nil {
			return err
		}
		fmt.Println("CID: ", ret.Root)
		fmt.Println("Piece size: ", ret.Size)
		return nil
	},
}

var clientCarGenCmd = &cli.Command{
	Name:      "generate-car",
	Usage:     "generate a car file from input",
	ArgsUsage: "[inputPath outputPath]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("usage: generate-car <inputPath> <outputPath>")
		}

		ref := lapi.FileRef{
			Path:  cctx.Args().First(),
			IsCAR: false,
		}

		op := cctx.Args().Get(1)

		if err = api.ClientGenCar(ctx, ref, op); err != nil {
			return err
		}
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
	Name:      "deal",
	Usage:     "Initialize storage deal with a miner",
	ArgsUsage: "[dataCid miner price duration]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manual-piece-cid",
			Usage: "manually specify piece commitment for data (dataCid must be to a car file)",
		},
		&cli.Int64Flag{
			Name:  "manual-piece-size",
			Usage: "if manually specifying piece cid, used to specify size (dataCid must be to a car file)",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "specify address to fund the deal with",
		},
		&cli.Int64Flag{
			Name:  "start-epoch",
			Usage: "specify the epoch that the deal should start at",
			Value: -1,
		},
	},
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

		// [data, miner, price, dur]

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

		var a address.Address
		if from := cctx.String("from"); from != "" {
			faddr, err := address.NewFromString(from)
			if err != nil {
				return xerrors.Errorf("failed to parse 'from' address: %w", err)
			}
			a = faddr
		} else {
			def, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			a = def
		}

		ref := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         data,
		}

		if mpc := cctx.String("manual-piece-cid"); mpc != "" {
			c, err := cid.Parse(mpc)
			if err != nil {
				return xerrors.Errorf("failed to parse provided manual piece cid: %w", err)
			}

			ref.PieceCid = &c

			psize := cctx.Int64("manual-piece-size")
			if psize == 0 {
				return xerrors.Errorf("must specify piece size when manually setting cid")
			}

			ref.PieceSize = abi.UnpaddedPieceSize(psize)

			ref.TransferType = storagemarket.TTManual
		}

		proposal, err := api.ClientStartDeal(ctx, &lapi.StartDealParams{
			Data:              ref,
			Wallet:            a,
			Miner:             miner,
			EpochPrice:        types.BigInt(price),
			MinBlocksDuration: uint64(dur),
			DealStartEpoch:    abi.ChainEpoch(cctx.Int64("start-epoch")),
		})
		if err != nil {
			return err
		}

		fmt.Println(proposal)
		return nil
	},
}

var clientFindCmd = &cli.Command{
	Name:      "find",
	Usage:     "find data in the network",
	ArgsUsage: "[dataCid]",
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
	Name:      "retrieve",
	Usage:     "retrieve data from network",
	ArgsUsage: "[dataCid outputPath]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "address",
			Usage: "address to use for transactions",
		},
		&cli.BoolFlag{
			Name:  "car",
			Usage: "export to a car file instead of a regular file",
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

		ref := lapi.FileRef{
			Path:  cctx.Args().Get(1),
			IsCAR: cctx.Bool("car"),
		}
		if err := api.ClientRetrieve(ctx, offers[0].Order(payer), ref); err != nil {
			return xerrors.Errorf("Retrieval Failed: %w", err)
		}

		fmt.Println("Success")
		return nil
	},
}

var clientQueryAskCmd = &cli.Command{
	Name:      "query-ask",
	Usage:     "find a miners ask",
	ArgsUsage: "[minerAddress]",
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
			p, err := peer.Decode(pidstr)
			if err != nil {
				return err
			}
			pid = p
		} else {
			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("failed to get peerID for miner: %w", err)
			}

			if mi.PeerId == peer.ID("SETME") {
				return fmt.Errorf("the miner hasn't initialized yet")
			}

			pid = mi.PeerId
		}

		ask, err := api.ClientQueryAsk(ctx, pid, maddr)
		if err != nil {
			return err
		}

		fmt.Printf("Ask: %s\n", maddr)
		fmt.Printf("Price per GiB: %s\n", types.FIL(ask.Ask.Price))
		fmt.Printf("Max Piece size: %d\n", ask.Ask.MaxPieceSize)

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

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		localDeals, err := api.ClientListDeals(ctx)
		if err != nil {
			return err
		}

		var deals []deal
		for _, v := range localDeals {
			if v.DealID == 0 {
				deals = append(deals, deal{
					LocalDeal: v,
					OnChainDealState: market.DealState{
						SectorStartEpoch: -1,
						LastUpdatedEpoch: -1,
						SlashEpoch:       -1,
					},
				})
			} else {
				onChain, err := api.StateMarketStorageDeal(ctx, v.DealID, head.Key())
				if err != nil {
					return err
				}

				deals = append(deals, deal{
					LocalDeal:        v,
					OnChainDealState: onChain.State,
				})
			}
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintf(w, "DealCid\tDealId\tProvider\tState\tOn Chain?\tSlashed?\tPieceCID\tSize\tPrice\tDuration\tMessage\n")
		for _, d := range deals {
			onChain := "N"
			if d.OnChainDealState.SectorStartEpoch != -1 {
				onChain = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SectorStartEpoch)
			}

			slashed := "N"
			if d.OnChainDealState.SlashEpoch != -1 {
				slashed = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SlashEpoch)
			}

			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\n", d.LocalDeal.ProposalCid, d.LocalDeal.DealID, d.LocalDeal.Provider, storagemarket.DealStates[d.LocalDeal.State], onChain, slashed, d.LocalDeal.PieceCID, d.LocalDeal.Size, d.LocalDeal.PricePerEpoch, d.LocalDeal.Duration, d.LocalDeal.Message)
		}
		return w.Flush()
	},
}

type deal struct {
	LocalDeal        lapi.DealInfo
	OnChainDealState market.DealState
}
