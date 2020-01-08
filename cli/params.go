package cli

import (
	"github.com/docker/go-units"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
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
		err = paramfetch.GetParams(build.ParametersJson, sectorSize)
		if err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		return nil
	},
}
