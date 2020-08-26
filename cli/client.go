package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	tm "github.com/buger/goterm"
	"github.com/docker/go-units"
	"github.com/fatih/color"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil/cidenc"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var CidBaseFlag = cli.StringFlag{
	Name:        "cid-base",
	Hidden:      true,
	Value:       "base32",
	Usage:       "Multibase encoding used for version 1 CIDs in output.",
	DefaultText: "base32",
}

// GetCidEncoder returns an encoder using the `cid-base` flag if provided, or
// the default (Base32) encoder if not.
func GetCidEncoder(cctx *cli.Context) (cidenc.Encoder, error) {
	val := cctx.String("cid-base")

	e := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}

	if val != "" {
		var err error
		e.Base, err = multibase.EncoderByName(val)
		if err != nil {
			return e, err
		}
	}

	return e, nil
}

var clientCmd = &cli.Command{
	Name:  "client",
	Usage: "Make deals, store data, retrieve data",
	Subcommands: []*cli.Command{
		WithCategory("storage", clientDealCmd),
		WithCategory("storage", clientQueryAskCmd),
		WithCategory("storage", clientListDeals),
		WithCategory("storage", clientGetDealCmd),
		WithCategory("data", clientImportCmd),
		WithCategory("data", clientDropCmd),
		WithCategory("data", clientLocalCmd),
		WithCategory("retrieval", clientFindCmd),
		WithCategory("retrieval", clientRetrieveCmd),
		WithCategory("util", clientCommPCmd),
		WithCategory("util", clientCarGenCmd),
		WithCategory("util", clientInfoCmd),
		WithCategory("util", clientListTransfers),
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
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Output root CID only",
		},
		&CidBaseFlag,
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return xerrors.New("expected input path as the only arg")
		}

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

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		if !cctx.Bool("quiet") {
			fmt.Printf("Import %d, Root ", c.ImportID)
		}
		fmt.Println(encoder.Encode(c.Root))

		return nil
	},
}

var clientDropCmd = &cli.Command{
	Name:      "drop",
	Usage:     "Remove import",
	ArgsUsage: "[import ID...]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("no imports specified")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var ids []multistore.StoreID
		for i, s := range cctx.Args().Slice() {
			id, err := strconv.ParseInt(s, 10, 0)
			if err != nil {
				return xerrors.Errorf("parsing %d-th import ID: %w", i, err)
			}

			ids = append(ids, multistore.StoreID(id))
		}

		for _, id := range ids {
			if err := api.ClientRemoveImport(ctx, id); err != nil {
				return xerrors.Errorf("removing import %d: %w", id, err)
			}
		}

		return nil
	},
}

var clientCommPCmd = &cli.Command{
	Name:      "commP",
	Usage:     "Calculate the piece-cid (commP) of a CAR file",
	ArgsUsage: "[inputFile]",
	Flags: []cli.Flag{
		&CidBaseFlag,
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 1 {
			return fmt.Errorf("usage: commP <inputPath>")
		}

		ret, err := api.ClientCalcCommP(ctx, cctx.Args().Get(0))
		if err != nil {
			return err
		}

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		fmt.Println("CID: ", encoder.Encode(ret.Root))
		fmt.Println("Piece size: ", types.SizeStr(types.NewInt(uint64(ret.Size))))
		return nil
	},
}

var clientCarGenCmd = &cli.Command{
	Name:      "generate-car",
	Usage:     "Generate a car file from input",
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
	Flags: []cli.Flag{
		&CidBaseFlag,
	},
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

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i].Key < list[j].Key
		})

		for _, v := range list {
			cidStr := "<nil>"
			if v.Root != nil {
				cidStr = encoder.Encode(*v.Root)
			}

			fmt.Printf("%d: %s @%s (%s)\n", v.Key, cidStr, v.FilePath, v.Source)
			if v.Err != "" {
				fmt.Printf("\terror: %s\n", v.Err)
			}
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
		&cli.BoolFlag{
			Name:  "fast-retrieval",
			Usage: "indicates that data should be available for fast retrieval",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "verified-deal",
			Usage: "indicate that the deal counts towards verified client total",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "provider-collateral",
			Usage: "specify the requested provider collateral the miner should put up",
		},
		&CidBaseFlag,
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return interactiveDeal(cctx)
		}

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

		var provCol big.Int
		if pcs := cctx.String("provider-collateral"); pcs != "" {
			pc, err := big.FromString(pcs)
			if err != nil {
				return fmt.Errorf("failed to parse provider-collateral: %w", err)
			}
			provCol = pc
		}

		if abi.ChainEpoch(dur) < build.MinDealDuration {
			return xerrors.Errorf("minimum deal duration is %d blocks", build.MinDealDuration)
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

		// Check if the address is a verified client
		dcap, err := api.StateVerifiedClientStatus(ctx, a, types.EmptyTSK)
		if err != nil {
			return err
		}

		isVerified := dcap != nil

		// If the user has explicitly set the --verified-deal flag
		if cctx.IsSet("verified-deal") {
			// If --verified-deal is true, but the address is not a verified
			// client, return an error
			verifiedDealParam := cctx.Bool("verified-deal")
			if verifiedDealParam && !isVerified {
				return xerrors.Errorf("address %s does not have verified client status", a)
			}

			// Override the default
			isVerified = verifiedDealParam
		}

		proposal, err := api.ClientStartDeal(ctx, &lapi.StartDealParams{
			Data:               ref,
			Wallet:             a,
			Miner:              miner,
			EpochPrice:         types.BigInt(price),
			MinBlocksDuration:  uint64(dur),
			DealStartEpoch:     abi.ChainEpoch(cctx.Int64("start-epoch")),
			FastRetrieval:      cctx.Bool("fast-retrieval"),
			VerifiedDeal:       isVerified,
			ProviderCollateral: provCol,
		})
		if err != nil {
			return err
		}

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		fmt.Println(encoder.Encode(*proposal))

		return nil
	},
}

func interactiveDeal(cctx *cli.Context) error {
	api, closer, err := GetFullNodeAPI(cctx)
	if err != nil {
		return err
	}
	defer closer()
	ctx := ReqContext(cctx)

	state := "import"

	var data cid.Cid
	var days int
	var maddr address.Address
	var ask storagemarket.StorageAsk
	var epochPrice big.Int
	var epochs abi.ChainEpoch

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

	printErr := func(err error) {
		fmt.Printf("%s %s\n", color.RedString("Error:"), err.Error())
	}

	for {
		// TODO: better exit handling
		if err := ctx.Err(); err != nil {
			return err
		}

		switch state {
		case "import":
			fmt.Print("Data CID (from " + color.YellowString("lotus client import") + "): ")

			var cidStr string
			_, err := fmt.Scan(&cidStr)
			if err != nil {
				printErr(xerrors.Errorf("reading cid string: %w", err))
				continue
			}

			data, err = cid.Parse(cidStr)
			if err != nil {
				printErr(xerrors.Errorf("parsing cid string: %w", err))
				continue
			}

			state = "duration"
		case "duration":
			fmt.Print("Deal duration (days): ")

			_, err := fmt.Scan(&days)
			if err != nil {
				printErr(xerrors.Errorf("parsing duration: %w", err))
				continue
			}

			state = "miner"
		case "miner":
			fmt.Print("Miner Address (t0..): ")
			var maddrStr string

			_, err := fmt.Scan(&maddrStr)
			if err != nil {
				printErr(xerrors.Errorf("reading miner address: %w", err))
				continue
			}

			maddr, err = address.NewFromString(maddrStr)
			if err != nil {
				printErr(xerrors.Errorf("parsing miner address: %w", err))
				continue
			}

			state = "query"
		case "query":
			color.Blue(".. querying miner ask")

			mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				printErr(xerrors.Errorf("failed to get peerID for miner: %w", err))
				state = "miner"
				continue
			}

			a, err := api.ClientQueryAsk(ctx, *mi.PeerId, maddr)
			if err != nil {
				printErr(xerrors.Errorf("failed to query ask: %w", err))
				state = "miner"
				continue
			}

			ask = *a.Ask

			// TODO: run more validation
			state = "confirm"
		case "confirm":
			fromBal, err := api.WalletBalance(ctx, a)
			if err != nil {
				return xerrors.Errorf("checking from address balance: %w", err)
			}

			color.Blue(".. calculating data size\n")
			ds, err := api.ClientDealSize(ctx, data)
			if err != nil {
				return err
			}

			dur := 24 * time.Hour * time.Duration(days)

			epochs = abi.ChainEpoch(dur / (time.Duration(build.BlockDelaySecs) * time.Second))
			// TODO: do some more or epochs math (round to miner PP, deal start buffer)

			gib := types.NewInt(1 << 30)

			// TODO: price is based on PaddedPieceSize, right?
			epochPrice = types.BigDiv(types.BigMul(ask.Price, types.NewInt(uint64(ds.PieceSize))), gib)
			totalPrice := types.BigMul(epochPrice, types.NewInt(uint64(epochs)))

			fmt.Printf("-----\n")
			fmt.Printf("Proposing from %s\n", a)
			fmt.Printf("\tBalance: %s\n", types.FIL(fromBal))
			fmt.Printf("\n")
			fmt.Printf("Piece size: %s (Payload size: %s)\n", units.BytesSize(float64(ds.PieceSize)), units.BytesSize(float64(ds.PayloadSize)))
			fmt.Printf("Duration: %s\n", dur)
			fmt.Printf("Total price: ~%s (%s per epoch)\n", types.FIL(totalPrice), types.FIL(epochPrice))

			state = "accept"
		case "accept":
			fmt.Print("\nAccept (yes/no): ")

			var yn string
			_, err := fmt.Scan(&yn)
			if err != nil {
				return err
			}

			if yn == "no" {
				return nil
			}

			if yn != "yes" {
				fmt.Println("Type in full 'yes' or 'no'")
				continue
			}

			state = "execute"
		case "execute":
			color.Blue(".. executing")
			proposal, err := api.ClientStartDeal(ctx, &lapi.StartDealParams{
				Data: &storagemarket.DataRef{
					TransferType: storagemarket.TTGraphsync,
					Root:         data,
				},
				Wallet:            a,
				Miner:             maddr,
				EpochPrice:        epochPrice,
				MinBlocksDuration: uint64(epochs),
				DealStartEpoch:    abi.ChainEpoch(cctx.Int64("start-epoch")),
				FastRetrieval:     cctx.Bool("fast-retrieval"),
				VerifiedDeal:      false, // TODO: Allow setting
			})
			if err != nil {
				return err
			}

			encoder, err := GetCidEncoder(cctx)
			if err != nil {
				return err
			}

			fmt.Println("\nDeal CID:", color.GreenString(encoder.Encode(*proposal)))
			return nil
		default:
			return xerrors.Errorf("unknown state: %s", state)
		}
	}
}

var clientFindCmd = &cli.Command{
	Name:      "find",
	Usage:     "Find data in the network",
	ArgsUsage: "[dataCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "pieceCid",
			Usage: "require data to be retrieved from a specific Piece CID",
		},
	},
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

		var pieceCid *cid.Cid
		if cctx.String("pieceCid") != "" {
			parsed, err := cid.Parse(cctx.String("pieceCid"))
			if err != nil {
				return err
			}
			pieceCid = &parsed
		}

		offers, err := api.ClientFindData(ctx, file, pieceCid)
		if err != nil {
			return err
		}

		for _, offer := range offers {
			if offer.Err != "" {
				fmt.Printf("ERR %s@%s: %s\n", offer.Miner, offer.MinerPeer.ID, offer.Err)
				continue
			}
			fmt.Printf("RETRIEVAL %s@%s-%s-%s\n", offer.Miner, offer.MinerPeer.ID, types.FIL(offer.MinPrice), types.SizeStr(types.NewInt(offer.Size)))
		}

		return nil
	},
}

const DefaultMaxRetrievePrice = 1

var clientRetrieveCmd = &cli.Command{
	Name:      "retrieve",
	Usage:     "Retrieve data from network",
	ArgsUsage: "[dataCid outputPath]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "address to send transactions from",
		},
		&cli.BoolFlag{
			Name:  "car",
			Usage: "export to a car file instead of a regular file",
		},
		&cli.StringFlag{
			Name:  "miner",
			Usage: "miner address for retrieval, if not present it'll use local discovery",
		},
		&cli.StringFlag{
			Name:  "maxPrice",
			Usage: fmt.Sprintf("maximum price the client is willing to consider (default: %d FIL)", DefaultMaxRetrievePrice),
		},
		&cli.StringFlag{
			Name:  "pieceCid",
			Usage: "require data to be retrieved from a specific Piece CID",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		fapi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var payer address.Address
		if cctx.String("from") != "" {
			payer, err = address.NewFromString(cctx.String("from"))
		} else {
			payer, err = fapi.WalletDefaultAddress(ctx)
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

		var pieceCid *cid.Cid
		if cctx.String("pieceCid") != "" {
			parsed, err := cid.Parse(cctx.String("pieceCid"))
			if err != nil {
				return err
			}
			pieceCid = &parsed
		}

		var offer api.QueryOffer
		minerStrAddr := cctx.String("miner")
		if minerStrAddr == "" { // Local discovery
			offers, err := fapi.ClientFindData(ctx, file, pieceCid)

			var cleaned []api.QueryOffer
			// filter out offers that errored
			for _, o := range offers {
				if o.Err == "" {
					cleaned = append(cleaned, o)
				}
			}

			offers = cleaned

			// sort by price low to high
			sort.Slice(offers, func(i, j int) bool {
				return offers[i].MinPrice.LessThan(offers[j].MinPrice)
			})
			if err != nil {
				return err
			}

			// TODO: parse offer strings from `client find`, make this smarter
			if len(offers) < 1 {
				fmt.Println("Failed to find file")
				return nil
			}
			offer = offers[0]
		} else { // Directed retrieval
			minerAddr, err := address.NewFromString(minerStrAddr)
			if err != nil {
				return err
			}
			offer, err = fapi.ClientMinerQueryOffer(ctx, minerAddr, file, pieceCid)
			if err != nil {
				return err
			}
		}
		if offer.Err != "" {
			return fmt.Errorf("The received offer errored: %s", offer.Err)
		}

		maxPrice := types.FromFil(DefaultMaxRetrievePrice)

		if cctx.String("maxPrice") != "" {
			maxPriceFil, err := types.ParseFIL(cctx.String("maxPrice"))
			if err != nil {
				return xerrors.Errorf("parsing maxPrice: %w", err)
			}

			maxPrice = types.BigInt(maxPriceFil)
		}

		if offer.MinPrice.GreaterThan(maxPrice) {
			return xerrors.Errorf("failed to find offer satisfying maxPrice: %s", maxPrice)
		}

		ref := &lapi.FileRef{
			Path:  cctx.Args().Get(1),
			IsCAR: cctx.Bool("car"),
		}
		updates, err := fapi.ClientRetrieveWithEvents(ctx, offer.Order(payer), ref)
		if err != nil {
			return xerrors.Errorf("error setting up retrieval: %w", err)
		}

		for {
			select {
			case evt, ok := <-updates:
				if ok {
					fmt.Printf("> Recv: %s, Paid %s, %s (%s)\n",
						types.SizeStr(types.NewInt(evt.BytesReceived)),
						types.FIL(evt.FundsSpent),
						retrievalmarket.ClientEvents[evt.Event],
						retrievalmarket.DealStatuses[evt.Status],
					)
				} else {
					fmt.Println("Success")
					return nil
				}

				if evt.Err != "" {
					return xerrors.Errorf("retrieval failed: %s", evt.Err)
				}
			case <-ctx.Done():
				return xerrors.Errorf("retrieval timed out")
			}
		}
	},
}

var clientQueryAskCmd = &cli.Command{
	Name:      "query-ask",
	Usage:     "Find a miners ask",
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
			fmt.Println("Usage: query-ask [minerAddress]")
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

			if *mi.PeerId == peer.ID("SETME") {
				return fmt.Errorf("the miner hasn't initialized yet")
			}

			pid = *mi.PeerId
		}

		ask, err := api.ClientQueryAsk(ctx, pid, maddr)
		if err != nil {
			return err
		}

		fmt.Printf("Ask: %s\n", maddr)
		fmt.Printf("Price per GiB: %s\n", types.FIL(ask.Ask.Price))
		fmt.Printf("Verified Price per GiB: %s\n", types.FIL(ask.Ask.VerifiedPrice))
		fmt.Printf("Max Piece size: %s\n", types.SizeStr(types.NewInt(uint64(ask.Ask.MaxPieceSize))))

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
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "print verbose deal details",
		},
		&cli.BoolFlag{
			Name:  "color",
			Usage: "use color in display output",
			Value: true,
		},
	},
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

		sort.Slice(localDeals, func(i, j int) bool {
			return localDeals[i].CreationTime.Before(localDeals[j].CreationTime)
		})

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
					deals = append(deals, deal{LocalDeal: v})
				} else {
					deals = append(deals, deal{
						LocalDeal:        v,
						OnChainDealState: onChain.State,
					})
				}
			}
		}

		color := cctx.Bool("color")

		if cctx.Bool("verbose") {
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintf(w, "Created\tDealCid\tDealId\tProvider\tState\tOn Chain?\tSlashed?\tPieceCID\tSize\tPrice\tDuration\tMessage\n")
			for _, d := range deals {
				onChain := "N"
				if d.OnChainDealState.SectorStartEpoch != -1 {
					onChain = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SectorStartEpoch)
				}

				slashed := "N"
				if d.OnChainDealState.SlashEpoch != -1 {
					slashed = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SlashEpoch)
				}

				price := types.FIL(types.BigMul(d.LocalDeal.PricePerEpoch, types.NewInt(d.LocalDeal.Duration)))
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", d.LocalDeal.CreationTime.Format(time.Stamp), d.LocalDeal.ProposalCid, d.LocalDeal.DealID, d.LocalDeal.Provider, dealStateString(color, d.LocalDeal.State), onChain, slashed, d.LocalDeal.PieceCID, types.SizeStr(types.NewInt(d.LocalDeal.Size)), price, d.LocalDeal.Duration, d.LocalDeal.Message)
			}
			return w.Flush()
		}

		w := tablewriter.New(tablewriter.Col("DealCid"),
			tablewriter.Col("DealId"),
			tablewriter.Col("Provider"),
			tablewriter.Col("State"),
			tablewriter.Col("On Chain?"),
			tablewriter.Col("Slashed?"),
			tablewriter.Col("PieceCID"),
			tablewriter.Col("Size"),
			tablewriter.Col("Price"),
			tablewriter.Col("Duration"),
			tablewriter.NewLineCol("Message"))

		for _, d := range deals {
			propcid := ellipsis(d.LocalDeal.ProposalCid.String(), 8)

			onChain := "N"
			if d.OnChainDealState.SectorStartEpoch != -1 {
				onChain = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SectorStartEpoch)
			}

			slashed := "N"
			if d.OnChainDealState.SlashEpoch != -1 {
				slashed = fmt.Sprintf("Y (epoch %d)", d.OnChainDealState.SlashEpoch)
			}

			piece := ellipsis(d.LocalDeal.PieceCID.String(), 8)

			price := types.FIL(types.BigMul(d.LocalDeal.PricePerEpoch, types.NewInt(d.LocalDeal.Duration)))

			w.Write(map[string]interface{}{
				"DealCid":   propcid,
				"DealId":    d.LocalDeal.DealID,
				"Provider":  d.LocalDeal.Provider,
				"State":     dealStateString(color, d.LocalDeal.State),
				"On Chain?": onChain,
				"Slashed?":  slashed,
				"PieceCID":  piece,
				"Size":      types.SizeStr(types.NewInt(d.LocalDeal.Size)),
				"Price":     price,
				"Duration":  d.LocalDeal.Duration,
				"Message":   d.LocalDeal.Message,
			})
		}

		return w.Flush(os.Stdout)
	},
}

func dealStateString(c bool, state storagemarket.StorageDealStatus) string {
	s := storagemarket.DealStates[state]
	if !c {
		return s
	}

	switch state {
	case storagemarket.StorageDealError, storagemarket.StorageDealExpired:
		return color.RedString(s)
	case storagemarket.StorageDealActive:
		return color.GreenString(s)
	default:
		return s
	}
}

type deal struct {
	LocalDeal        lapi.DealInfo
	OnChainDealState market.DealState
}

var clientGetDealCmd = &cli.Command{
	Name:  "get-deal",
	Usage: "Print detailed deal information",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		propcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		di, err := api.ClientGetDealInfo(ctx, propcid)
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"DealInfo: ": di,
		}

		if di.DealID != 0 {
			onChain, err := api.StateMarketStorageDeal(ctx, di.DealID, types.EmptyTSK)
			if err != nil {
				return err
			}

			out["OnChain"] = onChain
		}

		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

var clientInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print storage market client information",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "client",
			Usage: "specify storage client address",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var addr address.Address
		if clientFlag := cctx.String("client"); clientFlag != "" {
			ca, err := address.NewFromString("client")
			if err != nil {
				return err
			}

			addr = ca
		} else {
			def, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			addr = def
		}

		balance, err := api.StateMarketBalance(ctx, addr, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("Client Market Info:\n")

		fmt.Printf("Locked Funds:\t%s\n", types.FIL(balance.Locked))
		fmt.Printf("Escrowed Funds:\t%s\n", types.FIL(balance.Escrow))

		return nil
	},
}

var clientListTransfers = &cli.Command{
	Name:  "list-transfers",
	Usage: "List ongoing data transfers for deals",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "color",
			Usage: "use color in display output",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "completed",
			Usage: "show completed data transfers",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "watch deal updates in real-time, rather than a one time list",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		channels, err := api.ClientListDataTransfers(ctx)
		if err != nil {
			return err
		}

		completed := cctx.Bool("completed")
		color := cctx.Bool("color")
		watch := cctx.Bool("watch")

		if watch {
			channelUpdates, err := api.ClientDataTransferUpdates(ctx)
			if err != nil {
				return err
			}

			for {
				tm.Clear() // Clear current screen

				tm.MoveCursor(1, 1)

				OutputDataTransferChannels(tm.Screen, channels, completed, color)

				tm.Flush()

				select {
				case <-ctx.Done():
					return nil
				case channelUpdate := <-channelUpdates:
					var found bool
					for i, existing := range channels {
						if existing.TransferID == channelUpdate.TransferID &&
							existing.OtherPeer == channelUpdate.OtherPeer &&
							existing.IsSender == channelUpdate.IsSender &&
							existing.IsInitiator == channelUpdate.IsInitiator {
							channels[i] = channelUpdate
							found = true
							break
						}
					}
					if !found {
						channels = append(channels, channelUpdate)
					}
				}
			}
		}
		OutputDataTransferChannels(os.Stdout, channels, completed, color)
		return nil
	},
}

// OutputDataTransferChannels generates table output for a list of channels
func OutputDataTransferChannels(out io.Writer, channels []lapi.DataTransferChannel, completed bool, color bool) {
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].TransferID < channels[j].TransferID
	})

	var receivingChannels, sendingChannels []lapi.DataTransferChannel
	for _, channel := range channels {
		if !completed && channel.Status == datatransfer.Completed {
			continue
		}
		if channel.IsSender {
			sendingChannels = append(sendingChannels, channel)
		} else {
			receivingChannels = append(receivingChannels, channel)
		}
	}

	fmt.Fprintf(out, "Sending Channels\n\n")
	w := tablewriter.New(tablewriter.Col("ID"),
		tablewriter.Col("Status"),
		tablewriter.Col("Sending To"),
		tablewriter.Col("Root Cid"),
		tablewriter.Col("Initiated?"),
		tablewriter.Col("Transferred"),
		tablewriter.Col("Voucher"),
		tablewriter.NewLineCol("Message"))
	for _, channel := range sendingChannels {
		w.Write(toChannelOutput(color, "Sending To", channel))
	}
	w.Flush(out) //nolint:errcheck

	fmt.Fprintf(out, "\nReceiving Channels\n\n")
	w = tablewriter.New(tablewriter.Col("ID"),
		tablewriter.Col("Status"),
		tablewriter.Col("Receiving From"),
		tablewriter.Col("Root Cid"),
		tablewriter.Col("Initiated?"),
		tablewriter.Col("Transferred"),
		tablewriter.Col("Voucher"),
		tablewriter.NewLineCol("Message"))
	for _, channel := range receivingChannels {
		w.Write(toChannelOutput(color, "Receiving From", channel))
	}
	w.Flush(out) //nolint:errcheck
}

func channelStatusString(useColor bool, status datatransfer.Status) string {
	s := datatransfer.Statuses[status]
	if !useColor {
		return s
	}

	switch status {
	case datatransfer.Failed, datatransfer.Cancelled:
		return color.RedString(s)
	case datatransfer.Completed:
		return color.GreenString(s)
	default:
		return s
	}
}

func toChannelOutput(useColor bool, otherPartyColumn string, channel lapi.DataTransferChannel) map[string]interface{} {
	rootCid := ellipsis(channel.BaseCID.String(), 8)
	otherParty := ellipsis(channel.OtherPeer.String(), 8)

	initiated := "N"
	if channel.IsInitiator {
		initiated = "Y"
	}

	voucher := channel.Voucher
	if len(voucher) > 40 {
		voucher = ellipsis(voucher, 37)
	}

	return map[string]interface{}{
		"ID":             channel.TransferID,
		"Status":         channelStatusString(useColor, channel.Status),
		otherPartyColumn: otherParty,
		"Root Cid":       rootCid,
		"Initiated?":     initiated,
		"Transferred":    channel.Transferred,
		"Voucher":        voucher,
		"Message":        channel.Message,
	}
}

func ellipsis(s string, length int) string {
	if length > 0 && len(s) > length {
		return "..." + s[len(s)-length:]
	}
	return s
}
