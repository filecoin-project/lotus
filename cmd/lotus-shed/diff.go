package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

var diffCmd = &cli.Command{
	Name:  "diff",
	Usage: "diff state objects",
	Subcommands: []*cli.Command{
		diffStateTrees,
		diffMinerStates,
	},
}

var diffMinerStates = &cli.Command{
	Name:      "miner-states",
	Usage:     "diff two miner-states",
	ArgsUsage: "<stateCidA> <stateCidB>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		stCidA, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		stCidB, err := cid.Decode(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		actorStore := store.ActorStore(ctx, bs)

		var minerStA miner9.State
		if err = actorStore.Get(ctx, stCidA, &minerStA); err != nil {
			return err
		}

		var minerStB miner9.State
		if err = actorStore.Get(ctx, stCidB, &minerStB); err != nil {
			return err
		}

		fmt.Println(minerStA.Deadlines)
		fmt.Println(minerStB.Deadlines)

		minerDeadlinesA, err := minerStA.LoadDeadlines(actorStore)
		if err != nil {
			return err
		}

		minerDeadlinesB, err := minerStB.LoadDeadlines(actorStore)
		if err != nil {
			return err
		}

		for i, dACid := range minerDeadlinesA.Due {
			dBCid := minerDeadlinesB.Due[i]
			if dACid != dBCid {
				fmt.Println("Difference at index ", i, dACid, " != ", dBCid)
				dA, err := minerDeadlinesA.LoadDeadline(actorStore, uint64(i))
				if err != nil {
					return err
				}

				dB, err := minerDeadlinesB.LoadDeadline(actorStore, uint64(i))
				if err != nil {
					return err
				}

				if dA.SectorsSnapshot != dB.SectorsSnapshot {
					fmt.Println("They differ at Sectors snapshot ", dA.SectorsSnapshot, " != ", dB.SectorsSnapshot)

					sectorsSnapshotA, err := miner9.LoadSectors(actorStore, dA.SectorsSnapshot)
					if err != nil {
						return err
					}
					sectorsSnapshotB, err := miner9.LoadSectors(actorStore, dB.SectorsSnapshot)
					if err != nil {
						return err
					}

					if sectorsSnapshotA.Length() != sectorsSnapshotB.Length() {
						fmt.Println("sector snapshots have different lengts!")
					}

					var infoA miner9.SectorOnChainInfo
					err = sectorsSnapshotA.ForEach(&infoA, func(i int64) error {
						infoB, ok, err := sectorsSnapshotB.Get(abi.SectorNumber(i))
						if err != nil {
							return err
						}

						if !ok {
							fmt.Println(i, "isn't found in infoB!!")
						}

						if !infoA.DealWeight.Equals(infoB.DealWeight) {
							fmt.Println("Deal Weights differ! ", infoA.DealWeight, infoB.DealWeight)
						}

						if !infoA.VerifiedDealWeight.Equals(infoB.VerifiedDealWeight) {
							fmt.Println("Verified Deal Weights differ! ", infoA.VerifiedDealWeight, infoB.VerifiedDealWeight)
						}

						infoStrA := fmt.Sprint(infoA)
						infoStrB := fmt.Sprint(*infoB)
						if infoStrA != infoStrB {
							fmt.Println(infoStrA)
							fmt.Println(infoStrB)
						}

						return nil
					})
					if err != nil {
						return err
					}

				}
			}
		}

		return nil
	},
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
			return lcli.IncorrectNumArgs(cctx)
		}

		argA := cctx.Args().Get(0)
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
