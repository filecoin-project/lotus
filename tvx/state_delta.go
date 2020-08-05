package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var deltaFlags struct {
	from string
	to   string
}

var deltaCmd = &cli.Command{
	Name:        "state-delta",
	Description: "collect affected state between two tipsets, addressed by blocks",
	Action:      runStateDelta,
	Flags: []cli.Flag{
		&apiFlag,
		&cli.StringFlag{
			Name:        "from",
			Usage:       "block CID of initial state",
			Required:    true,
			Destination: &deltaFlags.from,
		},
		&cli.StringFlag{
			Name:        "to",
			Usage:       "block CID of ending state",
			Required:    true,
			Destination: &deltaFlags.to,
		},
	},
}

func runStateDelta(c *cli.Context) error {
	node, err := makeClient(c)
	if err != nil {
		return err
	}

	from, err := cid.Decode(deltaFlags.from)
	if err != nil {
		return err
	}

	to, err := cid.Decode(deltaFlags.to)
	if err != nil {
		return err
	}

	currBlock, err := node.ChainGetBlock(context.TODO(), to)
	if err != nil {
		return err
	}
	srcBlock, err := node.ChainGetBlock(context.TODO(), from)
	if err != nil {
		return err
	}

	allMsgs := make(map[uint64][]*types.Message)

	epochs := currBlock.Height - srcBlock.Height - 1
	for epochs > 0 {
		msgs, err := node.ChainGetBlockMessages(context.TODO(), to)
		if err != nil {
			return err
		}
		allMsgs[uint64(currBlock.Height)] = msgs.BlsMessages
		currBlock, err = node.ChainGetBlock(context.TODO(), currBlock.Parents[0])
		epochs--
	}

	if !hasParent(currBlock, from) {
		return fmt.Errorf("from block was not a parent of `to` as expected")
	}

	m := 0
	for _, msgs := range allMsgs {
		m += len(msgs)
	}

	fmt.Printf("messages: %d\n", m)
	fmt.Printf("initial state root: %v\n", currBlock.ParentStateRoot)
	return nil
}

func hasParent(block *types.BlockHeader, parent cid.Cid) bool {
	for _, p := range block.Parents {
		if p.Equals(parent) {
			return true
		}
	}
	return false
}
