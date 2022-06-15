package main

import (
	"github.com/docker/go-units"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-paramfetch"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
)

var fetchParamCmd = &cli.Command{
	Name:  "fetch-params",
	Usage: "Fetch proving parameters",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "proving-params",
			Usage: "download params used creating proofs for given size, i.e. 32GiB",
		},
	},
	Action: func(cctx *cli.Context) error {
		sectorSizeInt, err := units.RAMInBytes(cctx.String("proving-params"))
		if err != nil {
			return err
		}
		sectorSize := uint64(sectorSizeInt)
		err = paramfetch.GetParams(lcli.ReqContext(cctx), build.ParametersJSON(), build.SrsJSON(), sectorSize)
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
