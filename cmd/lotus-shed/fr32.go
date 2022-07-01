package main

import (
	"io"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fr32"
)

var fr32Cmd = &cli.Command{
	Name:        "fr32",
	Description: "fr32 encode/decode",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "decode",
			Aliases: []string{"d"},
		},
	},
	Action: func(context *cli.Context) error {
		if context.Bool("decode") {
			st, err := os.Stdin.Stat()
			if err != nil {
				return err
			}

			pps := abi.PaddedPieceSize(st.Size())
			if pps == 0 {
				return xerrors.Errorf("zero size input")
			}

			if err := pps.Validate(); err != nil {
				return err
			}

			r, err := fr32.NewUnpadReader(os.Stdin, pps)
			if err != nil {
				return err
			}
			if _, err := io.Copy(os.Stdout, r); err != nil {
				return err
			}
			return nil
		}

		w := fr32.NewPadWriter(os.Stdout)
		if _, err := io.Copy(w, os.Stdin); err != nil {
			return err
		}
		return w.Close()
	},
}
