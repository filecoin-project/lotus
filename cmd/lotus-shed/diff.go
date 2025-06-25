package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-hamt-ipld/v3"
	"github.com/filecoin-project/go-state-types/abi"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/repo"
)

var diffCmd = &cli.Command{
	Name:  "diff",
	Usage: "diff state objects",
	Subcommands: []*cli.Command{
		diffStateTrees,
		diffMinerStates,
		diffHAMTs,
		diffAMTs,
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

		defer func(lkrepo repo.LockedRepo) {
			_ = lkrepo.Close()
		}(lkrepo)

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
						fmt.Println("sector snapshots have different lengths!")
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
			}
			fmt.Printf("  actor does not exist in second state-tree (%s)\n", rootB)
			fmt.Println()
			delete(changedB, addr)
		}
		for addr, stateB := range changedB {
			fmt.Println(addr)
			stateA, ok := changedA[addr]
			if ok {
				diff(stateA, stateB)
				continue
			}
			fmt.Printf("  actor does not exist in first state-tree (%s)\n", rootA)
			fmt.Println()
		}
		return nil
	},
}

var diffHAMTs = &cli.Command{
	Name:      "hamts",
	Usage:     "diff two HAMTs",
	ArgsUsage: "<hamt-a> <hamt-b>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "car-file",
			Usage: "write a car file with two hamts (use lotus-shed export-car)",
		},
		&cli.IntFlag{
			Name:  "bitwidth",
			Usage: "bitwidth of the HAMT",
			Value: 5,
		},
		&cli.StringFlag{
			Name:  "key-type",
			Usage: "type of the key",
			Value: "uint",
		},
	},
	Action: func(cctx *cli.Context) error {
		var bs blockstore.Blockstore = blockstore.NewMemorySync()

		if cctx.IsSet("car-file") {
			f, err := os.Open(cctx.String("car-file"))
			if err != nil {
				return err
			}
			defer func(f *os.File) {
				_ = f.Close()
			}(f)

			cr, err := car.NewCarReader(f)
			if err != nil {
				return err
			}

			for {
				blk, err := cr.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}

				if err := bs.Put(cctx.Context, blk); err != nil {
					return err
				}
			}
		} else {
			// use running node
			api, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return xerrors.Errorf("connect to full node: %w", err)
			}
			defer closer()

			bs = blockstore.NewAPIBlockstore(api)
		}

		cidA, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		cidB, err := cid.Parse(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		cst := cbor.NewCborStore(bs)

		var keyParser func(k string) (interface{}, error)
		switch cctx.String("key-type") {
		case "uint":
			keyParser = func(k string) (interface{}, error) {
				return abi.ParseUIntKey(k)
			}
		case "actor":
			keyParser = func(k string) (interface{}, error) {
				return address.NewFromBytes([]byte(k))
			}
		default:
			return fmt.Errorf("unknown key type: %s", cctx.String("key-type"))
		}

		diffs, err := hamt.Diff(cctx.Context, cst, cst, cidA, cidB, hamt.UseTreeBitWidth(cctx.Int("bitwidth")))
		if err != nil {
			return err
		}

		for _, d := range diffs {
			switch d.Type {
			case hamt.Add:
				color.Green("+ Add %v", must.One(keyParser(d.Key)))
			case hamt.Remove:
				color.Red("- Remove %v", must.One(keyParser(d.Key)))
			case hamt.Modify:
				color.Yellow("~ Modify %v", must.One(keyParser(d.Key)))
			}
		}

		return nil
	},
}

var diffAMTs = &cli.Command{
	Name:      "amts",
	Usage:     "diff two AMTs",
	ArgsUsage: "<amt-a> <amt-b>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "car-file",
			Usage: "write a car file with two amts (use lotus-shed export-car)",
		},
		&cli.UintFlag{
			Name:  "bitwidth",
			Usage: "bitwidth of the AMT",
			Value: 5,
		},
	},
	Action: func(cctx *cli.Context) error {
		var bs blockstore.Blockstore = blockstore.NewMemorySync()

		if cctx.IsSet("car-file") {
			f, err := os.Open(cctx.String("car-file"))
			if err != nil {
				return err
			}
			defer func(f *os.File) {
				_ = f.Close()
			}(f)

			cr, err := car.NewCarReader(f)
			if err != nil {
				return err
			}

			for {
				blk, err := cr.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}

				if err := bs.Put(cctx.Context, blk); err != nil {
					return err
				}
			}
		} else {
			// use running node
			api, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return xerrors.Errorf("connect to full node: %w", err)
			}
			defer closer()

			bs = blockstore.NewAPIBlockstore(api)
		}

		cidA, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		cidB, err := cid.Parse(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		cst := cbor.NewCborStore(bs)

		diffs, err := amt.Diff(cctx.Context, cst, cst, cidA, cidB, amt.UseTreeBitWidth(cctx.Uint("bitwidth")))
		if err != nil {
			return err
		}

		for _, d := range diffs {
			switch d.Type {
			case amt.Add:
				color.Green("+ Add %v", d.Key)
			case amt.Remove:
				color.Red("- Remove %v", d.Key)
			case amt.Modify:
				color.Yellow("~ Modify %v", d.Key)

				var vb, va interface{}
				err := cbor.DecodeInto(d.Before.Raw, &vb)
				if err != nil {
					return err
				}
				err = cbor.DecodeInto(d.After.Raw, &va)
				if err != nil {
					return err
				}

				vjsonb, err := json.MarshalIndent(vb, " ", "  ")
				if err != nil {
					return err
				}
				vjsona, err := json.MarshalIndent(va, " ", "  ")
				if err != nil {
					return err
				}

				linesb := bytes.Split(vjsonb, []byte("\n")) // -
				linesa := bytes.Split(vjsona, []byte("\n")) // +

				maxLen := len(linesb)
				if len(linesa) > maxLen {
					maxLen = len(linesa)
				}

				for i := 0; i < maxLen; i++ {
					// Check if 'linesb' has run out of lines but 'linesa' hasn't
					if i >= len(linesb) && i < len(linesa) {
						color.Green("+ %s\n", linesa[i])
						continue
					}
					// Check if 'linesa' has run out of lines but 'linesb' hasn't
					if i >= len(linesa) && i < len(linesb) {
						color.Red("- %s\n", linesb[i])
						continue
					}
					// Compare lines if both slices have lines at index i
					if !bytes.Equal(linesb[i], linesa[i]) {
						color.Red("- %s\n", linesb[i])
						color.Green("+ %s\n", linesa[i])
					} else {
						// Print the line if it is the same in both slices
						fmt.Printf("  %s\n", linesb[i])
					}
				}

			}
		}

		return nil
	},
}
