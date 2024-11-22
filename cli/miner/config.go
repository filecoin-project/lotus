package miner

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		configUpdateCmd,
	},
}

var configDefaultCmd = &cli.Command{
	Name:  "default",
	Usage: "Print default node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		c := config.DefaultStorageMiner()

		cb, err := config.ConfigUpdate(c, nil, config.Commented(!cctx.Bool("no-comment")))
		if err != nil {
			return err
		}

		fmt.Println(string(cb))

		return nil
	},
}

var configUpdateCmd = &cli.Command{
	Name:  "updated",
	Usage: "Print updated node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String(FlagMinerRepo))
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}

		if !ok {
			return xerrors.Errorf("repo not initialized")
		}

		lr, err := r.LockRO(repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("locking repo: %w", err)
		}

		cfgNode, err := lr.Config()
		if err != nil {
			_ = lr.Close()
			return xerrors.Errorf("getting node config: %w", err)
		}

		if err := lr.Close(); err != nil {
			return err
		}

		cfgDef := config.DefaultStorageMiner()

		updated, err := config.ConfigUpdate(cfgNode, cfgDef, config.Commented(!cctx.Bool("no-comment")))
		if err != nil {
			return err
		}

		fmt.Print(string(updated))
		return nil
	},
}
