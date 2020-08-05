package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/urfave/cli/v2"
)

var apiFlag = cli.StringFlag{
	Name:    "api",
	Usage:   "api endpoint, formatted as token:multiaddr",
	Value:   "",
	EnvVars: []string{"FULLNODE_API_INFO"},
}

func main() {
	app := &cli.App{
		Name:        "tvx",
		Description: "a toolbox for managing test vectors",
		Usage:       "a toolbox for managing test vectors",
		Commands: []*cli.Command{
			deltaCmd,
			listAccessedCmd,
			extractMsgCmd,
			execLotusCmd,
			examineCmd,
			suiteMessagesCmd,
		},
	}

	sort.Sort(cli.CommandsByName(app.Commands))
	for _, c := range app.Commands {
		sort.Sort(cli.FlagsByName(c.Flags))
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func makeClient(c *cli.Context) (api.FullNode, error) {
	api := c.String(apiFlag.Name)
	sp := strings.SplitN(api, ":", 2)
	if len(sp) != 2 {
		return nil, fmt.Errorf("invalid api value, missing token or address: %s", api)
	}

	// TODO: discovery from filesystem
	token := sp[0]
	ma, err := multiaddr.NewMultiaddr(sp[1])
	if err != nil {
		return nil, fmt.Errorf("could not parse provided multiaddr: %w", err)
	}

	_, dialAddr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, fmt.Errorf("invalid api multiAddr: %w", err)
	}

	addr := "ws://" + dialAddr + "/rpc/v0"
	headers := http.Header{}
	if len(token) != 0 {
		headers.Add("Authorization", "Bearer "+token)
	}

	node, _, err := client.NewFullNodeRPC(addr, headers)
	if err != nil {
		return nil, fmt.Errorf("could not connect to api: %w", err)
	}
	return node, nil
}
