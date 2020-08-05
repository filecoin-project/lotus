package main

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/state"
)

var listAccessedFlags struct {
	cid string
}

var listAccessedCmd = &cli.Command{
	Name:        "list-accessed",
	Description: "extract actors accessed during the execution of a message",
	Action:      runListAccessed,
	Flags: []cli.Flag{
		&apiFlag,
		&cli.StringFlag{
			Name:        "cid",
			Usage:       "message CID",
			Required:    true,
			Destination: &listAccessedFlags.cid,
		},
	},
}

func runListAccessed(c *cli.Context) error {
	ctx := context.Background()

	node, err := makeClient(c)
	if err != nil {
		return err
	}

	mid, err := cid.Decode(listAccessedFlags.cid)
	if err != nil {
		return err
	}

	rtst := state.NewProxyingStore(ctx, node)

	sg := state.NewSurgeon(ctx, node, rtst)

	actors, err := sg.GetAccessedActors(context.TODO(), node, mid)
	if err != nil {
		return err
	}

	for k := range actors {
		fmt.Printf("%v\n", k)
	}
	return nil
}
