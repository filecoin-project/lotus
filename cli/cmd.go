package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/filecoin-project/go-lotus/api"
	"gopkg.in/urfave/cli.v2"
)

const (
	metadataContext = "context"
	metadataAPI     = "api"
)

// ApiConnector returns API instance
type ApiConnector func() api.API

func getApi(ctx *cli.Context) api.API {
	return ctx.App.Metadata[metadataAPI].(ApiConnector)()
}

// reqContext returns context for cli execution. Calling it for the first time
// installs SIGTERM handler that will close returned context.
// Not safe for concurrent execution.
func reqContext(cctx *cli.Context) context.Context {
	if uctx, ok := cctx.App.Metadata[metadataContext]; ok {
		// unchecked cast as if something else is in there
		// it is crash worthy either way
		return uctx.(context.Context)
	}
	ctx, done := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	go func() {
		<-sigChan
		done()
	}()
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	return ctx
}

var Commands = []*cli.Command{
	chainCmd,
	netCmd,
	versionCmd,
	mpoolCmd,
	minerCmd,
}
