package main

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/api/client"
	"log"
	"os"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/build"
	lcli "github.com/filecoin-project/go-lotus/cli"
	"github.com/filecoin-project/go-lotus/daemon"
)

func main() {
	local := []*cli.Command{
		daemon.Cmd,
	}

	app := &cli.App{
		Name:    "lotus",
		Usage:   "Filecoin decentralized storage network client",
		Version: build.Version,
		Metadata: map[string]interface{}{
			"api": lcli.ApiConnector(func() api.API {
				// TODO: get this from repo
				return client.NewRPC("http://127.0.0.1:1234/rpc/v0")
			}),
		},

		Commands: append(local, lcli.Commands...),
	}

	if err := app.Run(os.Args); err != nil {
		log.Println(err)
		return
	}
}
