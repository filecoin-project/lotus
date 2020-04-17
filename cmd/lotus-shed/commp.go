package main

import (
	"fmt"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"gopkg.in/urfave/cli.v2"
)

var commpToCidCmd = &cli.Command{
	Name:        "commp-to-cid",
	Description: "Convert a raw commP to a piece-Cid",
	Action: func(cctx *cli.Context) error {
		cp := []byte(cctx.Args().Get(0))
		fmt.Println(commcid.PieceCommitmentV1ToCID(cp))
		return nil
	},
}
