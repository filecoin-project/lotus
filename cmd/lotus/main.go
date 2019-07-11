package main

import (
	"log"
	"os"

	logging "github.com/ipfs/go-log"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/build"
	lcli "github.com/filecoin-project/go-lotus/cli"
	"github.com/filecoin-project/go-lotus/daemon"
)

func main() {
	logging.SetLogLevel("*", "INFO")
	local := []*cli.Command{
		daemon.Cmd,
	}

	app := &cli.App{
		Name:    "lotus",
		Usage:   "Filecoin decentralized storage network client",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: append(local, lcli.Commands...),
	}

	if err := app.Run(os.Args); err != nil {
		log.Println(err)
		return
	}
}
