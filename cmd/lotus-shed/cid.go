package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var cidCmd = &cli.Command{
	Name:  "cid",
	Usage: "Cid command",
	Subcommands: cli.Commands{
		cidIdCmd,
		inspectBundleCmd,
		cborCid,
		cidBytes,
	},
}

var cidBytes = &cli.Command{
	Name:      "bytes",
	Usage:     "cid bytes",
	ArgsUsage: "[cid]",
	Action: func(cctx *cli.Context) error {
		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}
		// Add in the troublesome zero byte prefix
		fmt.Printf("00%x\n", c.Bytes())
		return nil
	},
}

var cborCid = &cli.Command{
	Name:      "cbor",
	Usage:     "Serialize cid to cbor",
	ArgsUsage: "[cid]",
	Action: func(cctx *cli.Context) error {
		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}
		cbgc := cbg.CborCid(c)
		buf := bytes.NewBuffer(make([]byte, 0))
		if err := cbgc.MarshalCBOR(buf); err != nil {
			return err
		}
		fmt.Printf("%x\n", buf.Bytes())
		return nil
	},
}

var cidIdCmd = &cli.Command{
	Name:      "id",
	Usage:     "Create identity CID from hex or base64 data",
	ArgsUsage: "[data]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "encoding",
			Aliases: []string{"e"},
			Value:   "base64",
			Usage:   "specify input encoding to parse",
		},
		&cli.StringFlag{
			Name:  "codec",
			Value: "id",
			Usage: "multicodec-packed content types: abi or id",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify data")
		}

		var dec []byte
		switch cctx.String("encoding") {
		case "base64":
			data, err := base64.StdEncoding.DecodeString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("decoding base64 value: %w", err)
			}
			dec = data
		case "hex", "x":
			data, err := hex.DecodeString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("decoding hex value: %w", err)
			}
			dec = data
		case "raw", "r":
			dec = []byte(cctx.Args().First())
		default:
			return xerrors.Errorf("unrecognized encoding: %s", cctx.String("encoding"))
		}

		switch cctx.String("codec") {
		case "abi":
			aCid, err := abi.CidBuilder.Sum(dec)
			if err != nil {
				return xerrors.Errorf("cidBuilder abi: %w", err)
			}
			fmt.Println(aCid)
		case "id":
			builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
			rCid, err := builder.Sum(dec)
			if err != nil {
				return xerrors.Errorf("cidBuilder raw: %w", err)
			}
			fmt.Println(rCid)
		default:
			return xerrors.Errorf("unrecognized codec: %s", cctx.String("codec"))
		}

		return nil
	},
}

var inspectBundleCmd = &cli.Command{
	Name:      "inspect-bundle",
	Usage:     "Get the manifest CID from a car file, as well as the actor code CIDs",
	ArgsUsage: "[path]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		cf := cctx.Args().Get(0)
		f, err := os.OpenFile(cf, os.O_RDONLY, 0664)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}

		bs := blockstore.NewMemory()
		wrapBs := adt.WrapStore(ctx, cbor.NewCborStore(bs))

		hdr, err := car.LoadCar(ctx, bs, f)
		if err != nil {
			return xerrors.Errorf("error loading car file: %w", err)
		}

		manifestCid := hdr.Roots[0]

		fmt.Printf("Manifest CID: %s\n", manifestCid.String())

		entries, err := actors.ReadManifest(ctx, wrapBs, manifestCid)
		if err != nil {
			return xerrors.Errorf("error loading manifest: %w", err)
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "\nActor\tCID\t")

		for name, cid := range entries {
			_, _ = fmt.Fprintf(tw, "%v\t%v\n", name, cid)

		}

		return tw.Flush()
	},
}
