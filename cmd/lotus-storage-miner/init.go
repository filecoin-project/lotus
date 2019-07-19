package main

import (
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	lcli "github.com/filecoin-project/go-lotus/cli"
	"github.com/filecoin-project/go-lotus/node/repo"
)

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a lotus storage miner repo",
	Action: func(cctx *cli.Context) error {
		log.Info("Initializing lotus storage miner")
		log.Info("Checking if repo exists")

		r, err := repo.NewFS(cctx.String("storagerepo"))
		if err != nil {
			return err
		}

		if r.Exists() {
			return xerrors.Errorf("repo at '%s' is already initialized", cctx.String("storagerepo"))
		}

		log.Info("Trying to connect to full node RPC")

		api, err := lcli.GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := lcli.ReqContext(cctx)

		log.Info("Checking full node version")

		if err := r.Init(); err != nil {
			return err
		}

		return nil
	},
}
