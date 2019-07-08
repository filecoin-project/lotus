package cli

import (
	"github.com/filecoin-project/go-lotus/api"
	"gopkg.in/urfave/cli.v2"
)

type ApiConnector func() api.API

func getApi(ctx *cli.Context) api.API {
	return ctx.App.Metadata["api"].(ApiConnector)()
}

// Commands is the root group of CLI commands
var Commands = []*cli.Command{
	netCmd,
	versionCmd,
}
