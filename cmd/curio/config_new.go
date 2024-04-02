package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/node/repo"
)

var configNewCmd = &cli.Command{
	Name:      "new-cluster",
	Usage:     "Create new configuration for a new cluster",
	ArgsUsage: "[SP actor address...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "repo",
			EnvVars: []string{"LOTUS_PATH"},
			Hidden:  true,
			Value:   "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			return xerrors.New("must specify at least one SP actor address. Use 'lotus-shed miner create' or use 'curio guided-setup'")
		}

		ctx := cctx.Context

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		full, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return xerrors.Errorf("connecting to full node: %w", err)
		}
		defer closer()

		ainfo, err := cliutil.GetAPIInfo(cctx, repo.FullNode)
		if err != nil {
			return xerrors.Errorf("could not get API info for FullNode: %w", err)
		}

		token, err := full.AuthNew(ctx, api.AllPermissions)
		if err != nil {
			return err
		}

		return deps.CreateMinerConfig(ctx, full, db, cctx.Args().Slice(), fmt.Sprintf("%s:%s", string(token), ainfo.Addr))
	},
}
