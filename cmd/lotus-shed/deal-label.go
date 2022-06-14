package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v4/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/repo"
)

var dealLabelCmd = &cli.Command{
	Name:  "deal-label",
	Usage: "Scrape state to report on how many deals have non UTF-8 labels",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass state root")
		}

		sroot, err := cid.Decode(cctx.Args().First())
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

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		cst := cbor.NewCborStore(bs)
		store := adt.WrapStore(ctx, cst)

		tree, err := state.LoadStateTree(cst, sroot)
		if err != nil {
			return err
		}

		ma, err := tree.GetActor(market.Address)
		if err != nil {
			return err
		}

		ms, err := market.Load(store, ma)
		if err != nil {
			return err
		}

		ps, err := ms.Proposals()
		if err != nil {
			return err
		}

		var deals []abi.DealID
		if err = ps.ForEach(func(id abi.DealID, dp market.DealProposal) error {
			if dp.Label.IsBytes() {
				deals = append(deals, id)
			}

			return nil
		}); err != nil {
			return err
		}

		fmt.Println("there are ", len(deals), " bad labels")
		for _, d := range deals {
			fmt.Print(d, " ")
		}

		return nil
	},
}
