package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

var cidCmd = &cli.Command{
	Name: "cid",
	Subcommands: cli.Commands{
		cidIdCmd,
	},
}

var cidIdCmd = &cli.Command{
	Name:  "id",
	Usage: "create identity CID from hex or base64 data",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify data")
		}

		dec, err := hex.DecodeString(cctx.Args().First())
		if err != nil {
			dec, err = base64.StdEncoding.DecodeString(cctx.Args().First())
			if err != nil {
				return err
			}

		}

		builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}

		c, err := builder.Sum(dec)
		if err != nil {
			return err
		}

		fmt.Println(c)
		return nil
	},
}
