package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

var base64Cmd = &cli.Command{
	Name:        "base64",
	Description: "multiformats base64",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "decodeAddr",
			Value: false,
			Usage: "Decode a base64 addr",
		},
		&cli.BoolFlag{
			Name:  "decodeBig",
			Value: false,
			Usage: "Decode a base64 big",
		},
	},
	Action: func(cctx *cli.Context) error {
		var input io.Reader

		if cctx.NArg() == 0 {
			input = os.Stdin
		} else {
			input = strings.NewReader(cctx.Args().First())
		}

		bytes, err := io.ReadAll(input)
		if err != nil {
			return nil
		}

		decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(bytes)))
		if err != nil {
			return err
		}

		if cctx.Bool("decodeAddr") {
			addr, err := address.NewFromBytes(decoded)
			if err != nil {
				return err
			}

			fmt.Println(addr)

			return nil
		}

		if cctx.Bool("decodeBig") {
			var val abi.TokenAmount
			err = val.UnmarshalBinary(decoded)
			if err != nil {
				return err
			}

			fmt.Println(val)
		}

		return nil
	},
}
