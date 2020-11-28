package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"encoding/base64"
	"github.com/urfave/cli/v2"
)

var base64Cmd = &cli.Command{
	Name:        "base64",
	Usage:       "Base64 encode or decode",
	ArgsUsage:   "[base64]",
	Description: "encoding base64",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "decode",
			Aliases: []string{"d"},
			Value:   false,
			Usage:   "Decode the encoding base64",
		},
	},
	Action: func(cctx *cli.Context) error {
		var input io.Reader

		if cctx.Args().Len() == 0 {
			input = os.Stdin
		} else {
			input = strings.NewReader(cctx.Args().First())
		}

		bytes, err := ioutil.ReadAll(input)
		if err != nil {
			return nil
		}

		if cctx.Bool("decode") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(bytes)))
			if err != nil {
				return err
			}

			fmt.Println(string(decoded))
		} else {
			encoded := base64.StdEncoding.EncodeToString(bytes)
			fmt.Println(encoded)
		}

		return nil
	},
}
