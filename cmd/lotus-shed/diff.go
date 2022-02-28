package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var diffCmd = &cli.Command{
	Name:        "diff",
	Usage:       "diff state objects",
	Subcommands: []*cli.Command{diffStateTrees},
}

var diffStateTrees = &cli.Command{
	Name:      "state-trees",
	Usage:     "diff two state-trees",
	ArgsUsage: "<state-tree-a> <state-tree-b>",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 2 {
			return xerrors.Errorf("expected two state-tree roots")
		}

		argA := cctx.Args().Get(1)
		rootA, err := cid.Parse(argA)
		if err != nil {
			return xerrors.Errorf("first state-tree root (%q) is not a CID: %w", argA, err)
		}
		argB := cctx.Args().Get(1)
		rootB, err := cid.Parse(argB)
		if err != nil {
			return xerrors.Errorf("second state-tree root (%q) is not a CID: %w", argB, err)
		}

		if rootA == rootB {
			fmt.Println("state trees do not differ")
			return nil
		}

		changedB, err := api.StateChangedActors(ctx, rootA, rootB)
		if err != nil {
			return err
		}
		changedA, err := api.StateChangedActors(ctx, rootB, rootA)
		if err != nil {
			return err
		}

		diff := func(stateA, stateB types.Actor) {
			if stateB.Code != stateA.Code {
				fmt.Printf("  code: %s != %s\n", stateA.Code, stateB.Code)
			}
			if stateB.Head != stateA.Head {
				fmt.Printf("  state: %s != %s\n", stateA.Head, stateB.Head)
			}
			if stateB.Nonce != stateA.Nonce {
				fmt.Printf("  nonce: %d != %d\n", stateA.Nonce, stateB.Nonce)
			}
			if !stateB.Balance.Equals(stateA.Balance) {
				fmt.Printf("  balance: %s != %s\n", stateA.Balance, stateB.Balance)
			}
		}

		fmt.Printf("state differences between %s (first) and %s (second):\n\n", rootA, rootB)
		for addr, stateA := range changedA {
			fmt.Println(addr)
			stateB, ok := changedB[addr]
			if ok {
				diff(stateA, stateB)
				continue
			} else {
				fmt.Printf("  actor does not exist in second state-tree (%s)\n", rootB)
			}
			fmt.Println()
			delete(changedB, addr)
		}
		for addr, stateB := range changedB {
			fmt.Println(addr)
			stateA, ok := changedA[addr]
			if ok {
				diff(stateA, stateB)
				continue
			} else {
				fmt.Printf("  actor does not exist in first state-tree (%s)\n", rootA)
			}
			fmt.Println()
		}
		return nil
	},
}
