package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
)

var apiEndpoint string

var apiFlag = cli.StringFlag{
	Name:        "api",
	Usage:       "json-rpc api endpoint, formatted as [token]:multiaddr;" +
		"tvx uses unpriviliged operations, so the token may be omitted," +
		"but permissions may change in the future",
	EnvVars:     []string{"FULLNODE_API_INFO"},
	DefaultText: "",
	Destination: &apiEndpoint,
}

func main() {
	app := &cli.App{
		Name: "tvx",
		Description: `tvx is a tool for extracting and executing test vectors. It has two subcommands.

   tvx extract extracts a test vector from a live network. It requires access to
   a Filecoin client that exposes the standard JSON-RPC API endpoint. Set the API
   endpoint on the FULLNODE_API_INFO env variable, or through the --api flag. The
   format is token:multiaddr. Only message class test vectors are supported
   for now.

   tvx exec executes test vectors against Lotus. Either you can supply one in a
   file, or many as an ndjson stdin stream.`,
		Usage: "tvx is a tool for extracting and executing test vectors",
		Commands: []*cli.Command{
			extractCmd,
			execCmd,
		},
	}

	sort.Sort(cli.CommandsByName(app.Commands))
	for _, c := range app.Commands {
		sort.Sort(cli.FlagsByName(c.Flags))
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func makeAPIClient() (api.FullNode, jsonrpc.ClientCloser, error) {
	sp := strings.SplitN(apiEndpoint, ":", 2)
	if len(sp) != 2 {
		return nil, nil, fmt.Errorf("invalid api value, missing token or address: %s", apiEndpoint)
	}

	token := sp[0]
	ma, err := multiaddr.NewMultiaddr(sp[1])
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse provided multiaddr: %w", err)
	}

	_, dialAddr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid api multiAddr: %w", err)
	}

	var (
		addr    = "ws://" + dialAddr + "/rpc/v0"
		headers = make(http.Header, 1)
	)
	if len(token) != 0 {
		headers.Add("Authorization", "Bearer "+token)
	}

	node, closer, err := client.NewFullNodeRPC(context.Background(), addr, headers)
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to api: %w", err)
	}
	return node, closer, nil
}
