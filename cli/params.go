package cli

import (
	"github.com/filecoin-project/lotus/build"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
)

var fetchParamCmd = &cli.Command{
	Name:  "fetch-params",
	Usage: "Fetch proving parameters",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "proving-params",
			Usage: "download params used creating proofs for given size",
		},
	},
	Action: func(cctx *cli.Context) error {
		err := build.GetParams(cctx.Uint64("proving-params"))
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
