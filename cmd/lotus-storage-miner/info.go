package main

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	lcli "github.com/filecoin-project/go-lotus/cli"
)

var infoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print storage miner info",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		aaddr, err := nodeApi.ActorAddresses(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("actor address: %s\n", aaddr)
		// TODO: grab actr state / info
		//  * Sector size
		//  * Sealed sectors (count / bytes)
		//  * Power
		return nil
	},
}
