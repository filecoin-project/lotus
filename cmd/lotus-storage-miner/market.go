package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil/cidenc"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
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

var storageDealSelectionCmd = &cli.Command{
	Name:  "selection",
	Usage: "Configure acceptance criteria for storage deal proposals",
	Subcommands: []*cli.Command{
		storageDealSelectionShowCmd,
		storageDealSelectionResetCmd,
		storageDealSelectionRejectCmd,
	},
}

var storageDealSelectionShowCmd = &cli.Command{
	Name:  "list",
	Usage: "List storage deal proposal selection criteria",
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		onlineOk, err := smapi.DealsConsiderOnlineStorageDeals(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		offlineOk, err := smapi.DealsConsiderOfflineStorageDeals(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		fmt.Printf("considering online storage deals: %t\n", onlineOk)
		fmt.Printf("considering offline storage deals: %t\n", offlineOk)

		return nil
	},
}

var storageDealSelectionResetCmd = &cli.Command{
	Name:  "reset",
	Usage: "Reset storage deal proposal selection criteria to default values",
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		err = smapi.DealsSetConsiderOnlineStorageDeals(lcli.DaemonContext(cctx), true)
		if err != nil {
			return err
		}

		err = smapi.DealsSetConsiderOfflineStorageDeals(lcli.DaemonContext(cctx), true)
		if err != nil {
			return err
		}

		return nil
	},
}

var storageDealSelectionRejectCmd = &cli.Command{
	Name:  "reject",
	Usage: "Configure criteria which necessitate automatic rejection",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "online",
		},
		&cli.BoolFlag{
			Name: "offline",
		},
	},
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if cctx.Bool("online") {
			err = smapi.DealsSetConsiderOnlineStorageDeals(lcli.DaemonContext(cctx), false)
			if err != nil {
				return err
			}
		}

		if cctx.Bool("offline") {
			err = smapi.DealsSetConsiderOfflineStorageDeals(lcli.DaemonContext(cctx), false)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var setAskCmd = &cli.Command{
	Name:  "set-ask",
	Usage: "Configure the miner's ask",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "price",
			Usage:    "Set the price of the ask for unverified deals (specified as FIL / GiB / Epoch) to `PRICE`",
			Required: true,
		},
		&cli.Uint64Flag{
			Name:     "verified-price",
			Usage:    "Set the price of the ask for verified deals (specified as FIL / GiB / Epoch) to `PRICE`",
			Required: true,
		},
		&cli.StringFlag{
			Name:        "duration",
			Usage:       "Set duration of ask (a quantity of time after which the ask expires) `DURATION`",
			DefaultText: "720h0m0s",
			Value:       "720h0m0s",
		},
		&cli.StringFlag{
			Name:        "min-piece-size",
			Usage:       "Set minimum piece size (w/bit-padding, in bytes) in ask to `SIZE`",
			DefaultText: "256B",
			Value:       "256B",
		},
		&cli.StringFlag{
			Name:        "max-piece-size",
			Usage:       "Set maximum piece size (w/bit-padding, in bytes) in ask to `SIZE`",
			DefaultText: "miner sector size",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.DaemonContext(cctx)

		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		pri := types.NewInt(cctx.Uint64("price"))
		vpri := types.NewInt(cctx.Uint64("verified-price"))

		dur, err := time.ParseDuration(cctx.String("duration"))
		if err != nil {
			return xerrors.Errorf("cannot parse duration: %w", err)
		}

		qty := dur.Seconds() / float64(build.BlockDelaySecs)

		min, err := units.RAMInBytes(cctx.String("min-piece-size"))
		if err != nil {
			return xerrors.Errorf("cannot parse min-piece-size to quantity of bytes: %w", err)
		}

		if min < 256 {
			return xerrors.New("minimum piece size (w/bit-padding) is 256B")
		}

		max, err := units.RAMInBytes(cctx.String("max-piece-size"))
		if err != nil {
			return xerrors.Errorf("cannot parse max-piece-size to quantity of bytes: %w", err)
		}

		maddr, err := api.ActorAddress(ctx)
		if err != nil {
			return err
		}

		ssize, err := api.ActorSectorSize(ctx, maddr)
		if err != nil {
			return err
		}

		smax := int64(ssize)

		if max == 0 {
			max = smax
		}

		if max > smax {
			return xerrors.Errorf("max piece size (w/bit-padding) %s cannot exceed miner sector size %s", types.SizeStr(types.NewInt(uint64(max))), types.SizeStr(types.NewInt(uint64(smax))))
		}

		return api.MarketSetAsk(ctx, pri, vpri, abi.ChainEpoch(qty), abi.PaddedPieceSize(min), abi.PaddedPieceSize(max))
	},
}

var getAskCmd = &cli.Command{
	Name:  "get-ask",
	Usage: "Print the miner's ask",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.DaemonContext(cctx)

		fnapi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		sask, err := smapi.MarketGetAsk(ctx)
		if err != nil {
			return err
		}

		var ask *storagemarket.StorageAsk
		if sask != nil && sask.Ask != nil {
			ask = sask.Ask
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintf(w, "Price per GiB/Epoch\tVerified\tMin. Piece Size (padded)\tMax. Piece Size (padded)\tExpiry (Epoch)\tExpiry (Appx. Rem. Time)\tSeq. No.\n")
		if ask == nil {
			fmt.Fprintf(w, "<miner does not have an ask>\n")

			return w.Flush()
		}

		head, err := fnapi.ChainHead(ctx)
		if err != nil {
			return err
		}

		dlt := ask.Expiry - head.Height()
		rem := "<expired>"
		if dlt > 0 {
			rem = (time.Second * time.Duration(int64(dlt)*int64(build.BlockDelaySecs))).String()
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%d\n", ask.Price, ask.VerifiedPrice, types.SizeStr(types.NewInt(uint64(ask.MinPieceSize))), types.SizeStr(types.NewInt(uint64(ask.MaxPieceSize))), ask.Expiry, rem, ask.SeqNo)

		return w.Flush()
	},
}

var storageDealsCmd = &cli.Command{
	Name:  "storage-deals",
	Usage: "Manage storage deals and related configuration",
	Subcommands: []*cli.Command{
		dealsImportDataCmd,
		dealsListCmd,
		storageDealSelectionCmd,
		setAskCmd,
		getAskCmd,
		setBlocklistCmd,
		getBlocklistCmd,
		resetBlocklistCmd,
		setSealDurationCmd,
	},
}

var dealsImportDataCmd = &cli.Command{
	Name:      "import-data",
	Usage:     "Manually import data for a deal",
	ArgsUsage: "<proposal CID> <file>",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		if cctx.Args().Len() < 2 {
			return fmt.Errorf("must specify proposal CID and file path")
		}

		propCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		fpath := cctx.Args().Get(1)

		return api.DealsImportData(ctx, propCid, fpath)

	},
}

var dealsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List all deals for this miner",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		deals, err := api.MarketListIncompleteDeals(ctx)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)

		verbose := cctx.Bool("verbose")

		if verbose {
			_, _ = fmt.Fprintf(w, "ProposalCid\tDealId\tState\tClient\tSize\tPrice\tDuration\tMessage\n")
		} else {
			_, _ = fmt.Fprintf(w, "ProposalCid\tDealId\tState\tClient\tSize\tPrice\tDuration\n")
		}

		for _, deal := range deals {
			propcid := deal.ProposalCid.String()
			if !verbose {
				propcid = "..." + propcid[len(propcid)-8:]
			}

			fil := types.FIL(types.BigMul(deal.Proposal.StoragePricePerEpoch, types.NewInt(uint64(deal.Proposal.Duration()))))

			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s", propcid, deal.DealID, storagemarket.DealStates[deal.State], deal.Proposal.Client, units.BytesSize(float64(deal.Proposal.PieceSize)), fil, deal.Proposal.Duration())
			if verbose {
				_, _ = fmt.Fprintf(w, "\t%s", deal.Message)
			}

			_, _ = fmt.Fprintln(w)
		}

		return w.Flush()
	},
}

var getBlocklistCmd = &cli.Command{
	Name:  "get-blocklist",
	Usage: "List the contents of the miner's piece CID blocklist",
	Flags: []cli.Flag{
		&CidBaseFlag,
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		blocklist, err := api.DealsPieceCidBlocklist(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		encoder, err := GetCidEncoder(cctx)
		if err != nil {
			return err
		}

		for idx := range blocklist {
			fmt.Println(encoder.Encode(blocklist[idx]))
		}

		return nil
	},
}

var setBlocklistCmd = &cli.Command{
	Name:      "set-blocklist",
	Usage:     "Set the miner's list of blocklisted piece CIDs",
	ArgsUsage: "[<path-of-file-containing-newline-delimited-piece-CIDs> (optional, will read from stdin if omitted)]",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		scanner := bufio.NewScanner(os.Stdin)
		if cctx.Args().Present() && cctx.Args().First() != "-" {
			absPath, err := filepath.Abs(cctx.Args().First())
			if err != nil {
				return err
			}

			file, err := os.Open(absPath)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close() //nolint:errcheck

			scanner = bufio.NewScanner(file)
		}

		var blocklist []cid.Cid
		for scanner.Scan() {
			decoded, err := cid.Decode(scanner.Text())
			if err != nil {
				return err
			}

			blocklist = append(blocklist, decoded)
		}

		err = scanner.Err()
		if err != nil {
			return err
		}

		return api.DealsSetPieceCidBlocklist(lcli.DaemonContext(cctx), blocklist)
	},
}

var resetBlocklistCmd = &cli.Command{
	Name:  "reset-blocklist",
	Usage: "Remove all entries from the miner's piece CID blocklist",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		return api.DealsSetPieceCidBlocklist(lcli.DaemonContext(cctx), []cid.Cid{})
	},
}

var setSealDurationCmd = &cli.Command{
	Name:      "set-seal-duration",
	Usage:     "Set the expected time, in minutes, that you expect sealing sectors to take. Deals that start before this duration will be rejected.",
	ArgsUsage: "<minutes>",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass duration in minutes")
		}

		hs, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse duration: %w", err)
		}

		delay := hs * uint64(time.Minute)

		return nodeApi.SectorSetExpectedSealDuration(ctx, time.Duration(delay))
	},
}
