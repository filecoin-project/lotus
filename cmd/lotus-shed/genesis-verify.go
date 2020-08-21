package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ipfs/go-datastore"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
)

var genesisVerifyCmd = &cli.Command{
	Name:        "verify-genesis",
	Description: "verify some basic attributes of a genesis car file",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass genesis car file")
		}
		bs := blockstore.NewBlockstore(datastore.NewMapDatastore())

		cs := store.NewChainStore(bs, datastore.NewMapDatastore(), nil)

		cf := cctx.Args().Get(0)
		f, err := os.Open(cf)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}

		ts, err := cs.Import(f)
		if err != nil {
			return err
		}

		fmt.Println("File loaded, now verifying state tree balances...")

		sm := stmgr.NewStateManager(cs)

		total, err := stmgr.CheckTotalFIL(context.TODO(), sm, ts)
		if err != nil {
			return err
		}

		fmt.Printf("Total FIL: %s\n", types.FIL(total))
		return nil
	},
}
