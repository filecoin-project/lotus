package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var cidCmd = &cli.Command{
	Name:  "cid",
	Usage: "Cid command",
	Subcommands: cli.Commands{
		cidIdCmd,
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
