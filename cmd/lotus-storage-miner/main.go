package main

import (
	"os"

	logging "github.com/ipfs/go-log"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/build"
	lcli "github.com/filecoin-project/go-lotus/cli"
)

var log = logging.Logger("main")

func main() {
	logging.SetLogLevel("*", "INFO")
	local := []*cli.Command{
		RunCmd,
	}

	app := &cli.App{
		Name:    "lotus-storage-miner",
		Usage:   "Filecoin decentralized storage network storage miner",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "storagerepo",
				EnvVars: []string{"LOTUS_STORAGE_PATH"},
				Hidden:  true,
				Value:   "~/.lotusstorage", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: append(local, lcli.Commands...),
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err)
		return
	}
}
