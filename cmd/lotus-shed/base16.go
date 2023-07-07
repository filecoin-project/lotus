package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

var base16Cmd = &cli.Command{
	Name:        "base16",
	Description: "standard hex",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "decode",
			Value: false,
			Usage: "Decode the value",
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

		if cctx.Bool("decode") {
			decoded, err := hex.DecodeString(strings.TrimSpace(string(bytes)))
			if err != nil {
				return err
			}

			fmt.Println(string(decoded))
		} else {
			encoded := hex.EncodeToString(bytes)
			fmt.Println(encoded)
		}

		return nil
	},
}
