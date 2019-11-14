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
		&cli.BoolFlag{
			Name:  "only-verify-keys",
			Usage: "only download the verify keys",
		},
		&cli.BoolFlag{
			Name:  "include-test-params",
			Usage: "download params used for tests",
		},
	},
	Action: func(cctx *cli.Context) error {
		err := build.GetParams(!cctx.Bool("only-verify-keys"), cctx.Bool("include-test-params"))
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
