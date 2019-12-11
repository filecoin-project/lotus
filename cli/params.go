package cli

import (
	"strconv"
	"strings"

	"github.com/filecoin-project/lotus/build"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
)

var fetchParamCmd = &cli.Command{
	Name:  "fetch-params",
	Usage: "Fetch proving parameters",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "proving-params",
			Usage: "download params used creating proofs for given size",
		},
	},
	Action: func(cctx *cli.Context) error {
		var paramsize uint64
		pps := cctx.String("proving-params")
		if pps != "" {
			pps = strings.ToLower(pps)
			// TODO: replace with better parsing thing later
			switch pps {
			case "1gb":
				paramsize = 1 << 30
			case "32gb":
				paramsize = 32 << 30
			default:
				val, err := strconv.ParseUint(pps, 10, 64)
				if err != nil {
					return err
				}
				paramsize = val
			}
		}

		err := build.GetParams(paramsize)
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
