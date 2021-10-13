package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	tm "github.com/buger/goterm"
	"github.com/chzyer/readline"
	"github.com/docker/go-units"
	"github.com/fatih/color"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil/cidenc"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/filecoin-project/lotus/node/repo/imports"
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
		WithCategory("storage", clientListAsksCmd),
		WithCategory("storage", clientDealStatsCmd),
		WithCategory("storage", clientInspectDealCmd),
		WithCategory("data", clientImportCmd),
		WithCategory("data", clientDropCmd),
		WithCategory("data", clientLocalCmd),
		WithCategory("data", clientStat),
		WithCategory("retrieval", clientFindCmd),
		WithCategory("retrieval", clientRetrieveCmd),
		WithCategory("retrieval", clientCancelRetrievalDealCmd),
		WithCategory("retrieval", clientListRetrievalsCmd),
		WithCategory("util", clientCommPCmd),
		WithCategory("util", clientCarGenCmd),
		WithCategory("util", clientBalancesCmd),
		WithCategory("util", clientListTransfers),
		WithCategory("util", clientRestartTransfer),
		WithCategory("util", clientCancelTransfer),
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

		var ids []uint64
		for i, s := range cctx.Args().Slice() {
			id, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return xerrors.Errorf("parsing %d-th import ID: %w", i, err)
			}

			ids = append(ids, id)
		}

		for _, id := range ids {
			if err := api.ClientRemoveImport(ctx, imports.ID(id)); err != nil {
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
	Name:  "deal",
	Usage: "Initialize storage deal with a miner",
	Description: `Make a deal with a miner.
dataCid comes from running 'lotus client import'.
miner is the address of the miner you wish to make a deal with.
price is measured in FIL/Epoch. Miners usually don't accept a bid
lower than their advertised ask (which is in FIL/GiB/Epoch). You can check a miners listed price
with 'lotus client query-ask <miner address>'.
duration is how long the miner should store the data for, in blocks.
The minimum value is 518400 (6 months).`,
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
		&cli.BoolFlag{
			Name:  "manual-stateless-deal",
			Usage: "instructs the node to send an offline deal without registering it with the deallist/fsm",
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
			Name:        "verified-deal",
			Usage:       "indicate that the deal counts towards verified client total",
			DefaultText: "true if client is verified, false otherwise",
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
		afmt := NewAppFmt(cctx.App)

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
		if abi.ChainEpoch(dur) > build.MaxDealDuration {
			return xerrors.Errorf("maximum deal duration is %d blocks", build.MaxDealDuration)
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

		sdParams := &lapi.StartDealParams{
			Data:               ref,
			Wallet:             a,
			Miner:              miner,
			EpochPrice:         types.BigInt(price),
			MinBlocksDuration:  uint64(dur),
			DealStartEpoch:     abi.ChainEpoch(cctx.Int64("start-epoch")),
			FastRetrieval:      cctx.Bool("fast-retrieval"),
			VerifiedDeal:       isVerified,
			ProviderCollateral: provCol,
		}

		var proposal *cid.Cid
		if cctx.Bool("manual-stateless-deal") {
			if ref.TransferType != storagemarket.TTManual || price.Int64() != 0 {
				return xerrors.New("when manual-stateless-deal is enabled, you must also provide a 'price' of 0 and specify 'manual-piece-cid' and 'manual-piece-size'")
			}
			proposal, err = api.ClientStatelessDeal(ctx, sdParams)
		} else {
			proposal, err = api.ClientStartDeal(ctx, sdParams)
		}

		if err != nil {
			return err
		}

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		afmt.Println(encoder.Encode(*proposal))

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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	afmt := NewAppFmt(cctx.App)

	state := "import"
	gib := types.NewInt(1 << 30)

	var data cid.Cid
	var days int
	var maddrs []address.Address
	var ask []storagemarket.StorageAsk
	var epochPrices []big.Int
	var dur time.Duration
	var epochs abi.ChainEpoch
	var verified bool
	var ds lapi.DataCIDSize

	// find
	var candidateAsks []QueriedAsk
	var budget types.FIL
	var dealCount int64
	var medianPing, maxAcceptablePing time.Duration

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

	fromBal, err := api.WalletBalance(ctx, a)
	if err != nil {
		return xerrors.Errorf("checking from address balance: %w", err)
	}

	printErr := func(err error) {
		afmt.Printf("%s %s\n", color.RedString("Error:"), err.Error())
	}

	cs := readline.NewCancelableStdin(afmt.Stdin)
	go func() {
		<-ctx.Done()
		cs.Close() // nolint:errcheck
	}()

	rl := bufio.NewReader(cs)

uiLoop:
	for {
		// TODO: better exit handling
		if err := ctx.Err(); err != nil {
			return err
		}

		switch state {
		case "import":
			afmt.Print("Data CID (from " + color.YellowString("lotus client import") + "): ")

			_cidStr, _, err := rl.ReadLine()
			cidStr := string(_cidStr)
			if err != nil {
				printErr(xerrors.Errorf("reading cid string: %w", err))
				continue
			}

			data, err = cid.Parse(cidStr)
			if err != nil {
				printErr(xerrors.Errorf("parsing cid string: %w", err))
				continue
			}

			color.Blue(".. calculating data size\n")
			ds, err = api.ClientDealPieceCID(ctx, data)
			if err != nil {
				return err
			}

			state = "duration"
		case "duration":
			afmt.Print("Deal duration (days): ")

			_daystr, _, err := rl.ReadLine()
			daystr := string(_daystr)
			if err != nil {
				return err
			}

			_, err = fmt.Sscan(daystr, &days)
			if err != nil {
				printErr(xerrors.Errorf("parsing duration: %w", err))
				continue
			}

			if days < int(build.MinDealDuration/builtin.EpochsInDay) {
				printErr(xerrors.Errorf("minimum duration is %d days", int(build.MinDealDuration/builtin.EpochsInDay)))
				continue
			}

			dur = 24 * time.Hour * time.Duration(days)
			epochs = abi.ChainEpoch(dur / (time.Duration(build.BlockDelaySecs) * time.Second))

			state = "verified"
		case "verified":
			ts, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			dcap, err := api.StateVerifiedClientStatus(ctx, a, ts.Key())
			if err != nil {
				return err
			}

			if dcap == nil {
				state = "miner"
				continue
			}

			if dcap.Uint64() < uint64(ds.PieceSize) {
				color.Yellow(".. not enough DataCap available for a verified deal\n")
				state = "miner"
				continue
			}

			afmt.Print("\nMake this a verified deal? (yes/no): ")

			_yn, _, err := rl.ReadLine()
			yn := string(_yn)
			if err != nil {
				return err
			}

			switch yn {
			case "yes":
				verified = true
			case "no":
				verified = false
			default:
				afmt.Println("Type in full 'yes' or 'no'")
				continue
			}

			state = "miner"
		case "miner":
			afmt.Print("Miner Addresses (f0.. f0..), none to find: ")

			_maddrsStr, _, err := rl.ReadLine()
			maddrsStr := string(_maddrsStr)
			if err != nil {
				printErr(xerrors.Errorf("reading miner address: %w", err))
				continue
			}

			for _, s := range strings.Fields(maddrsStr) {
				maddr, err := address.NewFromString(strings.TrimSpace(s))
				if err != nil {
					printErr(xerrors.Errorf("parsing miner address: %w", err))
					continue uiLoop
				}

				maddrs = append(maddrs, maddr)
			}

			state = "query"
			if len(maddrs) == 0 {
				state = "find"
			}
		case "find":
			asks, err := GetAsks(ctx, api)
			if err != nil {
				return err
			}

			if len(asks) == 0 {
				printErr(xerrors.Errorf("no asks found"))
				continue uiLoop
			}

			medianPing = asks[len(asks)/2].Ping
			var avgPing time.Duration
			for _, ask := range asks {
				avgPing += ask.Ping
			}
			avgPing /= time.Duration(len(asks))

			for _, ask := range asks {
				if ask.Ask.MinPieceSize > ds.PieceSize {
					continue
				}
				if ask.Ask.MaxPieceSize < ds.PieceSize {
					continue
				}
				candidateAsks = append(candidateAsks, ask)
			}

			afmt.Printf("Found %d candidate asks\n", len(candidateAsks))
			afmt.Printf("Average network latency: %s; Median latency: %s\n", avgPing.Truncate(time.Millisecond), medianPing.Truncate(time.Millisecond))
			state = "max-ping"
		case "max-ping":
			maxAcceptablePing = medianPing

			afmt.Printf("Maximum network latency (default: %s) (ms): ", maxAcceptablePing.Truncate(time.Millisecond))
			_latStr, _, err := rl.ReadLine()
			latStr := string(_latStr)
			if err != nil {
				printErr(xerrors.Errorf("reading maximum latency: %w", err))
				continue
			}

			if latStr != "" {
				maxMs, err := strconv.ParseInt(latStr, 10, 64)
				if err != nil {
					printErr(xerrors.Errorf("parsing FIL: %w", err))
					continue uiLoop
				}

				maxAcceptablePing = time.Millisecond * time.Duration(maxMs)
			}

			var goodAsks []QueriedAsk
			for _, candidateAsk := range candidateAsks {
				if candidateAsk.Ping < maxAcceptablePing {
					goodAsks = append(goodAsks, candidateAsk)
				}
			}

			if len(goodAsks) == 0 {
				afmt.Printf("no asks left after filtering for network latency\n")
				continue uiLoop
			}

			afmt.Printf("%d asks left after filtering for network latency\n", len(goodAsks))
			candidateAsks = goodAsks

			state = "find-budget"
		case "find-budget":
			afmt.Printf("Proposing from %s, Current Balance: %s\n", a, types.FIL(fromBal))
			afmt.Print("Maximum budget (FIL): ") // TODO: Propose some default somehow?

			_budgetStr, _, err := rl.ReadLine()
			budgetStr := string(_budgetStr)
			if err != nil {
				printErr(xerrors.Errorf("reading miner address: %w", err))
				continue
			}

			budget, err = types.ParseFIL(budgetStr)
			if err != nil {
				printErr(xerrors.Errorf("parsing FIL: %w", err))
				continue uiLoop
			}

			var goodAsks []QueriedAsk
			for _, ask := range candidateAsks {
				p := ask.Ask.Price
				if verified {
					p = ask.Ask.VerifiedPrice
				}

				epochPrice := types.BigDiv(types.BigMul(p, types.NewInt(uint64(ds.PieceSize))), gib)
				totalPrice := types.BigMul(epochPrice, types.NewInt(uint64(epochs)))

				if totalPrice.LessThan(abi.TokenAmount(budget)) {
					goodAsks = append(goodAsks, ask)
				}
			}
			candidateAsks = goodAsks
			afmt.Printf("%d asks within budget\n", len(candidateAsks))
			state = "find-count"
		case "find-count":
			afmt.Print("Deals to make (1): ")
			dealcStr, _, err := rl.ReadLine()
			if err != nil {
				printErr(xerrors.Errorf("reading deal count: %w", err))
				continue
			}

			dealCount, err = strconv.ParseInt(string(dealcStr), 10, 64)
			if err != nil {
				return err
			}

			color.Blue(".. Picking miners")

			// TODO: some better strategy (this tries to pick randomly)
			var pickedAsks []*storagemarket.StorageAsk
		pickLoop:
			for i := 0; i < 64; i++ {
				rand.Shuffle(len(candidateAsks), func(i, j int) {
					candidateAsks[i], candidateAsks[j] = candidateAsks[j], candidateAsks[i]
				})

				remainingBudget := abi.TokenAmount(budget)
				pickedAsks = []*storagemarket.StorageAsk{}

				for _, ask := range candidateAsks {
					p := ask.Ask.Price
					if verified {
						p = ask.Ask.VerifiedPrice
					}

					epochPrice := types.BigDiv(types.BigMul(p, types.NewInt(uint64(ds.PieceSize))), gib)
					totalPrice := types.BigMul(epochPrice, types.NewInt(uint64(epochs)))

					if totalPrice.GreaterThan(remainingBudget) {
						continue
					}

					pickedAsks = append(pickedAsks, ask.Ask)
					remainingBudget = big.Sub(remainingBudget, totalPrice)

					if len(pickedAsks) == int(dealCount) {
						break pickLoop
					}
				}
			}

			for _, pickedAsk := range pickedAsks {
				maddrs = append(maddrs, pickedAsk.Miner)
				ask = append(ask, *pickedAsk)
			}

			state = "confirm"
		case "query":
			color.Blue(".. querying miner asks")

			for _, maddr := range maddrs {
				mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
				if err != nil {
					printErr(xerrors.Errorf("failed to get peerID for miner: %w", err))
					state = "miner"
					continue uiLoop
				}

				a, err := api.ClientQueryAsk(ctx, *mi.PeerId, maddr)
				if err != nil {
					printErr(xerrors.Errorf("failed to query ask: %w", err))
					state = "miner"
					continue uiLoop
				}

				ask = append(ask, *a)
			}

			// TODO: run more validation
			state = "confirm"
		case "confirm":
			// TODO: do some more or epochs math (round to miner PP, deal start buffer)

			afmt.Printf("-----\n")
			afmt.Printf("Proposing from %s\n", a)
			afmt.Printf("\tBalance: %s\n", types.FIL(fromBal))
			afmt.Printf("\n")
			afmt.Printf("Piece size: %s (Payload size: %s)\n", units.BytesSize(float64(ds.PieceSize)), units.BytesSize(float64(ds.PayloadSize)))
			afmt.Printf("Duration: %s\n", dur)

			pricePerGib := big.Zero()
			for _, a := range ask {
				p := a.Price
				if verified {
					p = a.VerifiedPrice
				}
				pricePerGib = big.Add(pricePerGib, p)
				epochPrice := types.BigDiv(types.BigMul(p, types.NewInt(uint64(ds.PieceSize))), gib)
				epochPrices = append(epochPrices, epochPrice)

				mpow, err := api.StateMinerPower(ctx, a.Miner, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting power (%s): %w", a.Miner, err)
				}

				if len(ask) > 1 {
					totalPrice := types.BigMul(epochPrice, types.NewInt(uint64(epochs)))
					afmt.Printf("Miner %s (Power:%s) price: ~%s (%s per epoch)\n", color.YellowString(a.Miner.String()), color.GreenString(types.SizeStr(mpow.MinerPower.QualityAdjPower)), color.BlueString(types.FIL(totalPrice).String()), types.FIL(epochPrice))
				}
			}

			// TODO: price is based on PaddedPieceSize, right?
			epochPrice := types.BigDiv(types.BigMul(pricePerGib, types.NewInt(uint64(ds.PieceSize))), gib)
			totalPrice := types.BigMul(epochPrice, types.NewInt(uint64(epochs)))

			afmt.Printf("Total price: ~%s (%s per epoch)\n", color.CyanString(types.FIL(totalPrice).String()), types.FIL(epochPrice))
			afmt.Printf("Verified: %v\n", verified)

			state = "accept"
		case "accept":
			afmt.Print("\nAccept (yes/no): ")

			_yn, _, err := rl.ReadLine()
			yn := string(_yn)
			if err != nil {
				return err
			}

			if yn == "no" {
				return nil
			}

			if yn != "yes" {
				afmt.Println("Type in full 'yes' or 'no'")
				continue
			}

			state = "execute"
		case "execute":
			color.Blue(".. executing\n")

			for i, maddr := range maddrs {
				proposal, err := api.ClientStartDeal(ctx, &lapi.StartDealParams{
					Data: &storagemarket.DataRef{
						TransferType: storagemarket.TTGraphsync,
						Root:         data,

						PieceCid:  &ds.PieceCID,
						PieceSize: ds.PieceSize.Unpadded(),
					},
					Wallet:            a,
					Miner:             maddr,
					EpochPrice:        epochPrices[i],
					MinBlocksDuration: uint64(epochs),
					DealStartEpoch:    abi.ChainEpoch(cctx.Int64("start-epoch")),
					FastRetrieval:     cctx.Bool("fast-retrieval"),
					VerifiedDeal:      verified,
				})
				if err != nil {
					return err
				}

				encoder, err := GetCidEncoder(cctx)
				if err != nil {
					return err
				}

				afmt.Printf("Deal (%s) CID: %s\n", maddr, color.GreenString(encoder.Encode(*proposal)))
			}

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

const DefaultMaxRetrievePrice = "0.01"

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
			Name:  "datamodel-path-selector",
			Usage: "a rudimentary (DM-level-only) text-path selector, allowing for sub-selection within a deal",
		},
		&cli.StringFlag{
			Name:  "maxPrice",
			Usage: fmt.Sprintf("maximum price the client is willing to consider (default: %s FIL)", DefaultMaxRetrievePrice),
		},
		&cli.StringFlag{
			Name:  "pieceCid",
			Usage: "require data to be retrieved from a specific Piece CID",
		},
		&cli.BoolFlag{
			Name: "allow-local",
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
		afmt := NewAppFmt(cctx.App)

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

		var pieceCid *cid.Cid
		if cctx.String("pieceCid") != "" {
			parsed, err := cid.Parse(cctx.String("pieceCid"))
			if err != nil {
				return err
			}
			pieceCid = &parsed
		}

		var order *lapi.RetrievalOrder
		if cctx.Bool("allow-local") {
			imports, err := fapi.ClientListImports(ctx)
			if err != nil {
				return err
			}

			for _, i := range imports {
				if i.Root != nil && i.Root.Equals(file) {
					order = &lapi.RetrievalOrder{
						Root:         file,
						FromLocalCAR: i.CARPath,

						Total:       big.Zero(),
						UnsealPrice: big.Zero(),
					}
					break
				}
			}
		}

		if order == nil {
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

			maxPrice := types.MustParseFIL(DefaultMaxRetrievePrice)

			if cctx.String("maxPrice") != "" {
				maxPrice, err = types.ParseFIL(cctx.String("maxPrice"))
				if err != nil {
					return xerrors.Errorf("parsing maxPrice: %w", err)
				}
			}

			if offer.MinPrice.GreaterThan(big.Int(maxPrice)) {
				return xerrors.Errorf("failed to find offer satisfying maxPrice: %s", maxPrice)
			}

			o := offer.Order(payer)
			order = &o
		}
		ref := &lapi.FileRef{
			Path:  cctx.Args().Get(1),
			IsCAR: cctx.Bool("car"),
		}

		if sel := textselector.Expression(cctx.String("datamodel-path-selector")); sel != "" {
			order.DatamodelPathSelector = &sel
		}

		updates, err := fapi.ClientRetrieveWithEvents(ctx, *order, ref)
		if err != nil {
			return xerrors.Errorf("error setting up retrieval: %w", err)
		}

		var prevStatus retrievalmarket.DealStatus

		for {
			select {
			case evt, ok := <-updates:
				if ok {
					afmt.Printf("> Recv: %s, Paid %s, %s (%s)\n",
						types.SizeStr(types.NewInt(evt.BytesReceived)),
						types.FIL(evt.FundsSpent),
						retrievalmarket.ClientEvents[evt.Event],
						retrievalmarket.DealStatuses[evt.Status],
					)
					prevStatus = evt.Status
				}

				if evt.Err != "" {
					return xerrors.Errorf("retrieval failed: %s", evt.Err)
				}

				if !ok {
					if prevStatus == retrievalmarket.DealStatusCompleted {
						afmt.Println("Success")
					} else {
						afmt.Printf("saw final deal state %s instead of expected success state DealStatusCompleted\n",
							retrievalmarket.DealStatuses[prevStatus])
					}
					return nil
				}

			case <-ctx.Done():
				return xerrors.Errorf("retrieval timed out")
			}
		}
	},
}

var clientListRetrievalsCmd = &cli.Command{
	Name:  "list-retrievals",
	Usage: "List retrieval market deals",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "print verbose deal details",
		},
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
		&cli.BoolFlag{
			Name:  "show-failed",
			Usage: "show failed/failing deals",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "completed",
			Usage: "show completed retrievals",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "watch deal updates in real-time, rather than a one time list",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		verbose := cctx.Bool("verbose")
		watch := cctx.Bool("watch")
		showFailed := cctx.Bool("show-failed")
		completed := cctx.Bool("completed")

		localDeals, err := api.ClientListRetrievals(ctx)
		if err != nil {
			return err
		}

		if watch {
			updates, err := api.ClientGetRetrievalUpdates(ctx)
			if err != nil {
				return err
			}

			for {
				tm.Clear()
				tm.MoveCursor(1, 1)

				err = outputRetrievalDeals(ctx, tm.Screen, localDeals, verbose, showFailed, completed)
				if err != nil {
					return err
				}

				tm.Flush()

				select {
				case <-ctx.Done():
					return nil
				case updated := <-updates:
					var found bool
					for i, existing := range localDeals {
						if existing.ID == updated.ID {
							localDeals[i] = updated
							found = true
							break
						}
					}
					if !found {
						localDeals = append(localDeals, updated)
					}
				}
			}
		}

		return outputRetrievalDeals(ctx, cctx.App.Writer, localDeals, verbose, showFailed, completed)
	},
}

func isTerminalError(status retrievalmarket.DealStatus) bool {
	// should patch this in go-fil-markets but to solve the problem immediate and not have buggy output
	return retrievalmarket.IsTerminalError(status) || status == retrievalmarket.DealStatusErrored || status == retrievalmarket.DealStatusCancelled
}
func outputRetrievalDeals(ctx context.Context, out io.Writer, localDeals []lapi.RetrievalInfo, verbose bool, showFailed bool, completed bool) error {
	var deals []api.RetrievalInfo
	for _, deal := range localDeals {
		if !showFailed && isTerminalError(deal.Status) {
			continue
		}
		if !completed && retrievalmarket.IsTerminalSuccess(deal.Status) {
			continue
		}
		deals = append(deals, deal)
	}

	tableColumns := []tablewriter.Column{
		tablewriter.Col("PayloadCID"),
		tablewriter.Col("DealId"),
		tablewriter.Col("Provider"),
		tablewriter.Col("Status"),
		tablewriter.Col("PricePerByte"),
		tablewriter.Col("Received"),
		tablewriter.Col("TotalPaid"),
	}

	if verbose {
		tableColumns = append(tableColumns,
			tablewriter.Col("PieceCID"),
			tablewriter.Col("UnsealPrice"),
			tablewriter.Col("BytesPaidFor"),
			tablewriter.Col("TransferChannelID"),
			tablewriter.Col("TransferStatus"),
		)
	}
	tableColumns = append(tableColumns, tablewriter.NewLineCol("Message"))

	w := tablewriter.New(tableColumns...)

	for _, d := range deals {
		w.Write(toRetrievalOutput(d, verbose))
	}

	return w.Flush(out)
}

func toRetrievalOutput(d api.RetrievalInfo, verbose bool) map[string]interface{} {

	payloadCID := d.PayloadCID.String()
	provider := d.Provider.String()
	if !verbose {
		payloadCID = ellipsis(payloadCID, 8)
		provider = ellipsis(provider, 8)
	}

	retrievalOutput := map[string]interface{}{
		"PayloadCID":   payloadCID,
		"DealId":       d.ID,
		"Provider":     provider,
		"Status":       retrievalStatusString(d.Status),
		"PricePerByte": types.FIL(d.PricePerByte),
		"Received":     units.BytesSize(float64(d.BytesReceived)),
		"TotalPaid":    types.FIL(d.TotalPaid),
		"Message":      d.Message,
	}

	if verbose {
		transferChannelID := ""
		if d.TransferChannelID != nil {
			transferChannelID = d.TransferChannelID.String()
		}
		transferStatus := ""
		if d.DataTransfer != nil {
			transferStatus = datatransfer.Statuses[d.DataTransfer.Status]
		}
		pieceCID := ""
		if d.PieceCID != nil {
			pieceCID = d.PieceCID.String()
		}

		retrievalOutput["PieceCID"] = pieceCID
		retrievalOutput["UnsealPrice"] = types.FIL(d.UnsealPrice)
		retrievalOutput["BytesPaidFor"] = units.BytesSize(float64(d.BytesPaidFor))
		retrievalOutput["TransferChannelID"] = transferChannelID
		retrievalOutput["TransferStatus"] = transferStatus
	}
	return retrievalOutput
}

func retrievalStatusString(status retrievalmarket.DealStatus) string {
	s := retrievalmarket.DealStatuses[status]

	switch {
	case isTerminalError(status):
		return color.RedString(s)
	case retrievalmarket.IsTerminalSuccess(status):
		return color.GreenString(s)
	default:
		return s
	}
}

var clientInspectDealCmd = &cli.Command{
	Name:  "inspect-deal",
	Usage: "Inspect detailed information about deal's lifecycle and the various stages it goes through",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name: "deal-id",
		},
		&cli.StringFlag{
			Name: "proposal-cid",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)
		return inspectDealCmd(ctx, api, cctx.String("proposal-cid"), cctx.Int("deal-id"))
	},
}

var clientDealStatsCmd = &cli.Command{
	Name:  "deal-stats",
	Usage: "Print statistics about local storage deals",
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Name: "newer-than",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		localDeals, err := api.ClientListDeals(ctx)
		if err != nil {
			return err
		}

		var totalSize uint64
		byState := map[storagemarket.StorageDealStatus][]uint64{}
		for _, deal := range localDeals {
			if cctx.IsSet("newer-than") {
				if time.Now().Sub(deal.CreationTime) > cctx.Duration("newer-than") {
					continue
				}
			}

			totalSize += deal.Size
			byState[deal.State] = append(byState[deal.State], deal.Size)
		}

		fmt.Printf("Total: %d deals, %s\n", len(localDeals), types.SizeStr(types.NewInt(totalSize)))

		type stateStat struct {
			state storagemarket.StorageDealStatus
			count int
			bytes uint64
		}

		stateStats := make([]stateStat, 0, len(byState))
		for state, deals := range byState {
			if state == storagemarket.StorageDealActive {
				state = math.MaxUint64 // for sort
			}

			st := stateStat{
				state: state,
				count: len(deals),
			}
			for _, b := range deals {
				st.bytes += b
			}

			stateStats = append(stateStats, st)
		}

		sort.Slice(stateStats, func(i, j int) bool {
			return int64(stateStats[i].state) < int64(stateStats[j].state)
		})

		for _, st := range stateStats {
			if st.state == math.MaxUint64 {
				st.state = storagemarket.StorageDealActive
			}
			fmt.Printf("%s: %d deals, %s\n", storagemarket.DealStates[st.state], st.count, types.SizeStr(types.NewInt(st.bytes)))
		}

		return nil
	},
}

var clientListAsksCmd = &cli.Command{
	Name:  "list-asks",
	Usage: "List asks for top miners",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "by-ping",
			Usage: "sort by ping",
		},
		&cli.StringFlag{
			Name:  "output-format",
			Value: "text",
			Usage: "Either 'text' or 'csv'",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		asks, err := GetAsks(ctx, api)
		if err != nil {
			return err
		}

		if cctx.Bool("by-ping") {
			sort.Slice(asks, func(i, j int) bool {
				return asks[i].Ping < asks[j].Ping
			})
		}
		pfmt := "%s: min:%s max:%s price:%s/GiB/Epoch verifiedPrice:%s/GiB/Epoch ping:%s\n"
		if cctx.String("output-format") == "csv" {
			fmt.Printf("Miner,Min,Max,Price,VerifiedPrice,Ping\n")
			pfmt = "%s,%s,%s,%s,%s,%s\n"
		}

		for _, a := range asks {
			ask := a.Ask

			fmt.Printf(pfmt, ask.Miner,
				types.SizeStr(types.NewInt(uint64(ask.MinPieceSize))),
				types.SizeStr(types.NewInt(uint64(ask.MaxPieceSize))),
				types.FIL(ask.Price),
				types.FIL(ask.VerifiedPrice),
				a.Ping,
			)
		}

		return nil
	},
}

type QueriedAsk struct {
	Ask  *storagemarket.StorageAsk
	Ping time.Duration
}

func GetAsks(ctx context.Context, api v0api.FullNode) ([]QueriedAsk, error) {
	isTTY := true
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		isTTY = false
	}
	if isTTY {
		color.Blue(".. getting miner list")
	}
	miners, err := api.StateListMiners(ctx, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting miner list: %w", err)
	}

	var lk sync.Mutex
	var found int64
	var withMinPower []address.Address
	done := make(chan struct{})

	go func() {
		defer close(done)

		var wg sync.WaitGroup
		wg.Add(len(miners))

		throttle := make(chan struct{}, 50)
		for _, miner := range miners {
			throttle <- struct{}{}
			go func(miner address.Address) {
				defer wg.Done()
				defer func() {
					<-throttle
				}()

				power, err := api.StateMinerPower(ctx, miner, types.EmptyTSK)
				if err != nil {
					return
				}

				if power.HasMinPower { // TODO: Lower threshold
					atomic.AddInt64(&found, 1)
					lk.Lock()
					withMinPower = append(withMinPower, miner)
					lk.Unlock()
				}
			}(miner)
		}
	}()

loop:
	for {
		select {
		case <-time.After(150 * time.Millisecond):
			if isTTY {
				fmt.Printf("\r* Found %d miners with power", atomic.LoadInt64(&found))
			}
		case <-done:
			break loop
		}
	}
	if isTTY {
		fmt.Printf("\r* Found %d miners with power\n", atomic.LoadInt64(&found))

		color.Blue(".. querying asks")
	}

	var asks []QueriedAsk
	var queried, got int64

	done = make(chan struct{})
	go func() {
		defer close(done)

		var wg sync.WaitGroup
		wg.Add(len(withMinPower))

		throttle := make(chan struct{}, 50)
		for _, miner := range withMinPower {
			throttle <- struct{}{}
			go func(miner address.Address) {
				defer wg.Done()
				defer func() {
					<-throttle
					atomic.AddInt64(&queried, 1)
				}()

				ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
				defer cancel()

				mi, err := api.StateMinerInfo(ctx, miner, types.EmptyTSK)
				if err != nil {
					return
				}
				if mi.PeerId == nil {
					return
				}

				ask, err := api.ClientQueryAsk(ctx, *mi.PeerId, miner)
				if err != nil {
					return
				}

				rt := time.Now()
				_, err = api.ClientQueryAsk(ctx, *mi.PeerId, miner)
				if err != nil {
					return
				}
				pingDuration := time.Now().Sub(rt)

				atomic.AddInt64(&got, 1)
				lk.Lock()
				asks = append(asks, QueriedAsk{
					Ask:  ask,
					Ping: pingDuration,
				})
				lk.Unlock()
			}(miner)
		}
	}()

loop2:
	for {
		select {
		case <-time.After(150 * time.Millisecond):
			if isTTY {
				fmt.Printf("\r* Queried %d asks, got %d responses", atomic.LoadInt64(&queried), atomic.LoadInt64(&got))
			}
		case <-done:
			break loop2
		}
	}
	if isTTY {
		fmt.Printf("\r* Queried %d asks, got %d responses\n", atomic.LoadInt64(&queried), atomic.LoadInt64(&got))
	}

	sort.Slice(asks, func(i, j int) bool {
		return asks[i].Ask.Price.LessThan(asks[j].Ask.Price)
	})

	return asks, nil
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
		afmt := NewAppFmt(cctx.App)
		if cctx.NArg() != 1 {
			afmt.Println("Usage: query-ask [minerAddress]")
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

			if mi.PeerId == nil || *mi.PeerId == peer.ID("SETME") {
				return fmt.Errorf("the miner hasn't initialized yet")
			}

			pid = *mi.PeerId
		}

		ask, err := api.ClientQueryAsk(ctx, pid, maddr)
		if err != nil {
			return err
		}

		afmt.Printf("Ask: %s\n", maddr)
		afmt.Printf("Price per GiB: %s\n", types.FIL(ask.Price))
		afmt.Printf("Verified Price per GiB: %s\n", types.FIL(ask.VerifiedPrice))
		afmt.Printf("Max Piece size: %s\n", types.SizeStr(types.NewInt(uint64(ask.MaxPieceSize))))
		afmt.Printf("Min Piece size: %s\n", types.SizeStr(types.NewInt(uint64(ask.MinPieceSize))))

		size := cctx.Int64("size")
		if size == 0 {
			return nil
		}
		perEpoch := types.BigDiv(types.BigMul(ask.Price, types.NewInt(uint64(size))), types.NewInt(1<<30))
		afmt.Printf("Price per Block: %s\n", types.FIL(perEpoch))

		duration := cctx.Int64("duration")
		if duration == 0 {
			return nil
		}
		afmt.Printf("Total Price: %s\n", types.FIL(types.BigMul(perEpoch, types.NewInt(uint64(duration)))))

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
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
		&cli.BoolFlag{
			Name:  "show-failed",
			Usage: "show failed/failing deals",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "watch deal updates in real-time, rather than a one time list",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		verbose := cctx.Bool("verbose")
		watch := cctx.Bool("watch")
		showFailed := cctx.Bool("show-failed")

		localDeals, err := api.ClientListDeals(ctx)
		if err != nil {
			return err
		}

		if watch {
			updates, err := api.ClientGetDealUpdates(ctx)
			if err != nil {
				return err
			}

			for {
				tm.Clear()
				tm.MoveCursor(1, 1)

				err = outputStorageDeals(ctx, tm.Screen, api, localDeals, verbose, showFailed)
				if err != nil {
					return err
				}

				tm.Flush()

				select {
				case <-ctx.Done():
					return nil
				case updated := <-updates:
					var found bool
					for i, existing := range localDeals {
						if existing.ProposalCid.Equals(updated.ProposalCid) {
							localDeals[i] = updated
							found = true
							break
						}
					}
					if !found {
						localDeals = append(localDeals, updated)
					}
				}
			}
		}

		return outputStorageDeals(ctx, cctx.App.Writer, api, localDeals, verbose, showFailed)
	},
}

func dealFromDealInfo(ctx context.Context, full v0api.FullNode, head *types.TipSet, v api.DealInfo) deal {
	if v.DealID == 0 {
		return deal{
			LocalDeal:        v,
			OnChainDealState: *market.EmptyDealState(),
		}
	}

	onChain, err := full.StateMarketStorageDeal(ctx, v.DealID, head.Key())
	if err != nil {
		return deal{LocalDeal: v}
	}

	return deal{
		LocalDeal:        v,
		OnChainDealState: onChain.State,
	}
}

func outputStorageDeals(ctx context.Context, out io.Writer, full v0api.FullNode, localDeals []lapi.DealInfo, verbose bool, showFailed bool) error {
	sort.Slice(localDeals, func(i, j int) bool {
		return localDeals[i].CreationTime.Before(localDeals[j].CreationTime)
	})

	head, err := full.ChainHead(ctx)
	if err != nil {
		return err
	}

	var deals []deal
	for _, localDeal := range localDeals {
		if showFailed || localDeal.State != storagemarket.StorageDealError {
			deals = append(deals, dealFromDealInfo(ctx, full, head, localDeal))
		}
	}

	if verbose {
		w := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
		fmt.Fprintf(w, "Created\tDealCid\tDealId\tProvider\tState\tOn Chain?\tSlashed?\tPieceCID\tSize\tPrice\tDuration\tTransferChannelID\tTransferStatus\tVerified\tMessage\n")
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
			transferChannelID := ""
			if d.LocalDeal.TransferChannelID != nil {
				transferChannelID = d.LocalDeal.TransferChannelID.String()
			}
			transferStatus := ""
			if d.LocalDeal.DataTransfer != nil {
				transferStatus = datatransfer.Statuses[d.LocalDeal.DataTransfer.Status]
				// TODO: Include the transferred percentage once this bug is fixed:
				// https://github.com/ipfs/go-graphsync/issues/126
				//fmt.Printf("transferred: %d / size: %d\n", d.LocalDeal.DataTransfer.Transferred, d.LocalDeal.Size)
				//if d.LocalDeal.Size > 0 {
				//	pct := (100 * d.LocalDeal.DataTransfer.Transferred) / d.LocalDeal.Size
				//	transferPct = fmt.Sprintf("%d%%", pct)
				//}
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%v\t%s\n",
				d.LocalDeal.CreationTime.Format(time.Stamp),
				d.LocalDeal.ProposalCid,
				d.LocalDeal.DealID,
				d.LocalDeal.Provider,
				dealStateString(d.LocalDeal.State),
				onChain,
				slashed,
				d.LocalDeal.PieceCID,
				types.SizeStr(types.NewInt(d.LocalDeal.Size)),
				price,
				d.LocalDeal.Duration,
				transferChannelID,
				transferStatus,
				d.LocalDeal.Verified,
				d.LocalDeal.Message)
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
		tablewriter.Col("Verified"),
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
			"State":     dealStateString(d.LocalDeal.State),
			"On Chain?": onChain,
			"Slashed?":  slashed,
			"PieceCID":  piece,
			"Size":      types.SizeStr(types.NewInt(d.LocalDeal.Size)),
			"Price":     price,
			"Verified":  d.LocalDeal.Verified,
			"Duration":  d.LocalDeal.Duration,
			"Message":   d.LocalDeal.Message,
		})
	}

	return w.Flush(out)
}

func dealStateString(state storagemarket.StorageDealStatus) string {
	s := storagemarket.DealStates[state]
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

var clientBalancesCmd = &cli.Command{
	Name:  "balances",
	Usage: "Print storage market client balances",
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
			ca, err := address.NewFromString(clientFlag)
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

		reserved, err := api.MarketGetReserved(ctx, addr)
		if err != nil {
			return err
		}

		avail := big.Sub(big.Sub(balance.Escrow, balance.Locked), reserved)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}

		fmt.Printf("Client Market Balance for address %s:\n", addr)

		fmt.Printf("  Escrowed Funds:        %s\n", types.FIL(balance.Escrow))
		fmt.Printf("  Locked Funds:          %s\n", types.FIL(balance.Locked))
		fmt.Printf("  Reserved Funds:        %s\n", types.FIL(reserved))
		fmt.Printf("  Available to Withdraw: %s\n", types.FIL(avail))

		return nil
	},
}

var clientStat = &cli.Command{
	Name:      "stat",
	Usage:     "Print information about a locally stored file (piece size, etc)",
	ArgsUsage: "<cid>",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() || cctx.NArg() != 1 {
			return fmt.Errorf("must specify cid of data")
		}

		dataCid, err := cid.Parse(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("parsing data cid: %w", err)
		}

		ds, err := api.ClientDealSize(ctx, dataCid)
		if err != nil {
			return err
		}

		fmt.Printf("Piece Size  : %v\n", ds.PieceSize)
		fmt.Printf("Payload Size: %v\n", ds.PayloadSize)

		return nil
	},
}

var clientRestartTransfer = &cli.Command{
	Name:  "restart-transfer",
	Usage: "Force restart a stalled data transfer",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "peerid",
			Usage: "narrow to transfer with specific peer",
		},
		&cli.BoolFlag{
			Name:  "initiator",
			Usage: "specify only transfers where peer is/is not initiator",
			Value: true,
		},
	},
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

		transferUint, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return fmt.Errorf("Error reading transfer ID: %w", err)
		}
		transferID := datatransfer.TransferID(transferUint)
		initiator := cctx.Bool("initiator")
		var other peer.ID
		if pidstr := cctx.String("peerid"); pidstr != "" {
			p, err := peer.Decode(pidstr)
			if err != nil {
				return err
			}
			other = p
		} else {
			channels, err := api.ClientListDataTransfers(ctx)
			if err != nil {
				return err
			}
			found := false
			for _, channel := range channels {
				if channel.IsInitiator == initiator && channel.TransferID == transferID {
					other = channel.OtherPeer
					found = true
					break
				}
			}
			if !found {
				return errors.New("unable to find matching data transfer")
			}
		}

		return api.ClientRestartDataTransfer(ctx, transferID, other, initiator)
	},
}

var clientCancelTransfer = &cli.Command{
	Name:  "cancel-transfer",
	Usage: "Force cancel a data transfer",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "peerid",
			Usage: "narrow to transfer with specific peer",
		},
		&cli.BoolFlag{
			Name:  "initiator",
			Usage: "specify only transfers where peer is/is not initiator",
			Value: true,
		},
		&cli.DurationFlag{
			Name:  "cancel-timeout",
			Usage: "time to wait for cancel to be sent to storage provider",
			Value: 5 * time.Second,
		},
	},
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

		transferUint, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return fmt.Errorf("Error reading transfer ID: %w", err)
		}
		transferID := datatransfer.TransferID(transferUint)
		initiator := cctx.Bool("initiator")
		var other peer.ID
		if pidstr := cctx.String("peerid"); pidstr != "" {
			p, err := peer.Decode(pidstr)
			if err != nil {
				return err
			}
			other = p
		} else {
			channels, err := api.ClientListDataTransfers(ctx)
			if err != nil {
				return err
			}
			found := false
			for _, channel := range channels {
				if channel.IsInitiator == initiator && channel.TransferID == transferID {
					other = channel.OtherPeer
					found = true
					break
				}
			}
			if !found {
				return errors.New("unable to find matching data transfer")
			}
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, cctx.Duration("cancel-timeout"))
		defer cancel()
		return api.ClientCancelDataTransfer(timeoutCtx, transferID, other, initiator)
	},
}

var clientCancelRetrievalDealCmd = &cli.Command{
	Name:  "cancel-retrieval",
	Usage: "Cancel a retrieval deal by deal ID; this also cancels the associated transfer",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:     "deal-id",
			Usage:    "specify retrieval deal by deal ID",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		id := cctx.Int64("deal-id")
		if id < 0 {
			return errors.New("deal id cannot be negative")
		}

		return api.ClientCancelRetrievalDeal(ctx, retrievalmarket.DealID(id))
	},
}

var clientListTransfers = &cli.Command{
	Name:  "list-transfers",
	Usage: "List ongoing data transfers for deals",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "print verbose transfer details",
		},
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
		&cli.BoolFlag{
			Name:  "completed",
			Usage: "show completed data transfers",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "watch deal updates in real-time, rather than a one time list",
		},
		&cli.BoolFlag{
			Name:  "show-failed",
			Usage: "show failed/cancelled transfers",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

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

		verbose := cctx.Bool("verbose")
		completed := cctx.Bool("completed")
		watch := cctx.Bool("watch")
		showFailed := cctx.Bool("show-failed")
		if watch {
			channelUpdates, err := api.ClientDataTransferUpdates(ctx)
			if err != nil {
				return err
			}

			for {
				tm.Clear() // Clear current screen

				tm.MoveCursor(1, 1)

				OutputDataTransferChannels(tm.Screen, channels, verbose, completed, showFailed)

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
		OutputDataTransferChannels(os.Stdout, channels, verbose, completed, showFailed)
		return nil
	},
}

// OutputDataTransferChannels generates table output for a list of channels
func OutputDataTransferChannels(out io.Writer, channels []lapi.DataTransferChannel, verbose, completed, showFailed bool) {
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].TransferID < channels[j].TransferID
	})

	var receivingChannels, sendingChannels []lapi.DataTransferChannel
	for _, channel := range channels {
		if !completed && channel.Status == datatransfer.Completed {
			continue
		}
		if !showFailed && (channel.Status == datatransfer.Failed || channel.Status == datatransfer.Cancelled) {
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
		w.Write(toChannelOutput("Sending To", channel, verbose))
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
		w.Write(toChannelOutput("Receiving From", channel, verbose))
	}
	w.Flush(out) //nolint:errcheck
}

func channelStatusString(status datatransfer.Status) string {
	s := datatransfer.Statuses[status]
	switch status {
	case datatransfer.Failed, datatransfer.Cancelled:
		return color.RedString(s)
	case datatransfer.Completed:
		return color.GreenString(s)
	default:
		return s
	}
}

func toChannelOutput(otherPartyColumn string, channel lapi.DataTransferChannel, verbose bool) map[string]interface{} {
	rootCid := channel.BaseCID.String()
	otherParty := channel.OtherPeer.String()
	if !verbose {
		rootCid = ellipsis(rootCid, 8)
		otherParty = ellipsis(otherParty, 8)
	}

	initiated := "N"
	if channel.IsInitiator {
		initiated = "Y"
	}

	voucher := channel.Voucher
	if len(voucher) > 40 && !verbose {
		voucher = ellipsis(voucher, 37)
	}

	return map[string]interface{}{
		"ID":             channel.TransferID,
		"Status":         channelStatusString(channel.Status),
		otherPartyColumn: otherParty,
		"Root Cid":       rootCid,
		"Initiated?":     initiated,
		"Transferred":    units.BytesSize(float64(channel.Transferred)),
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

func inspectDealCmd(ctx context.Context, api v0api.FullNode, proposalCid string, dealId int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	deals, err := api.ClientListDeals(ctx)
	if err != nil {
		return err
	}

	var di *lapi.DealInfo
	for i, cdi := range deals {
		if proposalCid != "" && cdi.ProposalCid.String() == proposalCid {
			di = &deals[i]
			break
		}

		if dealId != 0 && int(cdi.DealID) == dealId {
			di = &deals[i]
			break
		}
	}

	if di == nil {
		if proposalCid != "" {
			return fmt.Errorf("cannot find deal with proposal cid: %s", proposalCid)
		}
		if dealId != 0 {
			return fmt.Errorf("cannot find deal with deal id: %v", dealId)
		}
		return errors.New("you must specify proposal cid or deal id in order to inspect a deal")
	}

	// populate DealInfo.DealStages and DataTransfer.Stages
	di, err = api.ClientGetDealInfo(ctx, di.ProposalCid)
	if err != nil {
		return fmt.Errorf("cannot get deal info for proposal cid: %v", di.ProposalCid)
	}

	renderDeal(di)

	return nil
}

func renderDeal(di *lapi.DealInfo) {
	color.Blue("Deal ID:      %d\n", int(di.DealID))
	color.Blue("Proposal CID: %s\n\n", di.ProposalCid.String())

	if di.DealStages == nil {
		color.Yellow("Deal was made with an older version of Lotus and Lotus did not collect detailed information about its stages")
		return
	}

	for _, stg := range di.DealStages.Stages {
		msg := fmt.Sprintf("%s %s: %s (expected duration: %s)", color.BlueString("Stage:"), color.BlueString(strings.TrimPrefix(stg.Name, "StorageDeal")), stg.Description, color.GreenString(stg.ExpectedDuration))
		if stg.UpdatedTime.Time().IsZero() {
			msg = color.YellowString(msg)
		}
		fmt.Println(msg)

		for _, l := range stg.Logs {
			fmt.Printf("  %s %s\n", color.YellowString(l.UpdatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), l.Log)
		}

		if stg.Name == "StorageDealStartDataTransfer" {
			for _, dtStg := range di.DataTransfer.Stages.Stages {
				fmt.Printf("        %s %s %s\n", color.YellowString(dtStg.CreatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), color.BlueString("Data transfer stage:"), color.BlueString(dtStg.Name))
				for _, l := range dtStg.Logs {
					fmt.Printf("              %s %s\n", color.YellowString(l.UpdatedTime.Time().UTC().Round(time.Second).Format(time.Stamp)), l.Log)
				}
			}
		}
	}
}
