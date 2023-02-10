package main

import (
	"fmt"
	"reflect"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var ethCmd = &cli.Command{
	Name:        "eth",
	Description: "Ethereum compatibility related commands",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Subcommands: []*cli.Command{
		checkTipsetsCmd,
	},
}

var checkTipsetsCmd = &cli.Command{
	Name:        "check-tipsets",
	Description: "Check that eth_getBlockByNumber and eth_getBlockByHash consistently return tipsets",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		height := head.Height()
		fmt.Println("Current height:", height)
		for i := int64(height); i > 0; i-- {
			if _, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(i), types.EmptyTSK); err != nil {
				fmt.Printf("[FAIL] failed to get tipset @%d from Lotus: %s\n", i, err)
				continue
			}
			hex := fmt.Sprintf("0x%x", i)
			ethBlockA, err := api.EthGetBlockByNumber(ctx, hex, false)
			if err != nil {
				fmt.Printf("[FAIL] failed to get tipset @%d via eth_getBlockByNumber: %s\n", i, err)
				continue
			}
			ethBlockB, err := api.EthGetBlockByHash(ctx, ethBlockA.Hash, false)
			if err != nil {
				fmt.Printf("[FAIL] failed to get tipset @%d via eth_getBlockByHash: %s\n", i, err)
				continue
			}
			if equal := reflect.DeepEqual(ethBlockA, ethBlockB); equal {
				fmt.Printf("[OK] blocks received via eth_getBlockByNumber and eth_getBlockByHash for tipset @%d are identical\n", i)
			} else {
				fmt.Printf("[FAIL] blocks received via eth_getBlockByNumber and eth_getBlockByHash for tipset @%d are NOT identical\n", i)
			}
		}
		return nil
	},
}
