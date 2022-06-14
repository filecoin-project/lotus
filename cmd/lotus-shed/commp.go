package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	commcid "github.com/filecoin-project/go-fil-commcid"
)

var commpToCidCmd = &cli.Command{
	Name:        "commp-to-cid",
	Usage:       "Convert commP to Cid",
	Description: "Convert a raw commP to a piece-Cid",
	ArgsUsage:   "[data]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "encoding",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify commP to convert")
		}

		var dec []byte
		switch cctx.String("encoding") {
		case "base64":
			data, err := base64.StdEncoding.DecodeString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("decoding base64 value: %w", err)
			}
			dec = data
		case "hex":
			data, err := hex.DecodeString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("decoding hex value: %w", err)
			}
			dec = data
		default:
			return xerrors.Errorf("unrecognized encoding: %s", cctx.String("encoding"))
		}

		cid, err := commcid.PieceCommitmentV1ToCID(dec)
		if err != nil {
			return err
		}
		fmt.Println(cid)
		return nil
	},
}
