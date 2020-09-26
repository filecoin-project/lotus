package main

import (
	"encoding/hex"
	"fmt"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/urfave/cli/v2"
)

var commpToCidCmd = &cli.Command{
	Name:        "commp-to-cid",
	Description: "Convert a raw commP to a piece-Cid",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify commP to convert")
		}

		dec, err := hex.DecodeString(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to decode input as hex string: %w", err)
		}

		fmt.Println(commcid.PieceCommitmentV1ToCID(dec))
		return nil
	},
}
