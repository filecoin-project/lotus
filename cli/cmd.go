package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	manet "github.com/multiformats/go-multiaddr-net"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/api/client"
	"github.com/filecoin-project/go-lotus/node/repo"
)

const (
	metadataContext = "context"
)

// ApiConnector returns API instance
type ApiConnector func() api.API

func getAPI(ctx *cli.Context) (api.API, error) {
	r, err := repo.NewFS(ctx.String("repo"))
	if err != nil {
		return nil, err
	}

	ma, err := r.APIEndpoint()
	if err != nil {
		return nil, err
	}
	_, addr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, err
	}
	return client.NewRPC("ws://" + addr + "/rpc/v0")
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
	clientCmd,
	minerCmd,
	mpoolCmd,
	netCmd,
	versionCmd,
	walletCmd,
	createMinerCmd,

	{
		Name: "testch",
		Action: func(cctx *cli.Context) error {
			api, err := getAPI(cctx)
			if err != nil {
				return err
			}
			ctx := reqContext(cctx)

			c, err := api.TestCh(ctx)
			if err != nil {
				return err
			}

			for {
				select {
				case n := <-c:
					fmt.Println(n)
				case <-ctx.Done():
					return nil
				}
			}
		},
	},
}
