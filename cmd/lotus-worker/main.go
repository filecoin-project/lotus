package main

import (
	"os"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/cli/worker"
	"github.com/filecoin-project/lotus/lib/lotuslog"
)

var log = logging.Logger("main")

func main() {
	api.RunningNodeType = api.NodeWorker

	lotuslog.SetupLogLevels()

	app := worker.App()
	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}
