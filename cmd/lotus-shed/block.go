package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var blockCmd = &cli.Command{
	Name:      "block",
	Usage:     "Output decoded block header in readeble form",
	ArgsUsage: "[block header hex]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		b, err := hex.DecodeString(cctx.Args().First())
		if err != nil {
			return err
		}

		var blk types.BlockHeader
		if err := blk.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
			return err
		}

		jb, err := json.MarshalIndent(blk, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(jb))
		return nil
	},
}
