package cli

import (
	"github.com/filecoin-project/go-lotus/build"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
)

var fetchParamCmd = &cli.Command{
	Name:  "fetch-params",
	Usage: "Fetch proving parameters",
	Action: func(cctx *cli.Context) error {
		if err := build.GetParams(true); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
