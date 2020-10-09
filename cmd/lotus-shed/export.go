package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
)

var exportChainCmd = &cli.Command{
	Name:        "export",
	Description: "Export chain from repo (requires node to be offline)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset to export from",
		},
		&cli.Int64Flag{
			Name: "recent-stateroots",
		},
		&cli.BoolFlag{
			Name: "full-state",
		},
		&cli.BoolFlag{
			Name: "skip-old-msgs",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify file name to write export to"))
		}

		ctx := context.TODO()

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		fi, err := os.Create(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening the output file: %w", err)
		}

		defer fi.Close() //nolint:errcheck

		ds, err := lr.Datastore("/chain")
		if err != nil {
			return err
		}

		mds, err := lr.Datastore("/metadata")
		if err != nil {
			return err
		}

		bs := blockstore.NewBlockstore(ds)

		cs := store.NewChainStore(bs, mds, nil, nil)
		if err := cs.Load(); err != nil {
			return err
		}

		nroots := abi.ChainEpoch(cctx.Int64("recent-stateroots"))
		fullstate := cctx.Bool("full-state")
		skipoldmsgs := cctx.Bool("skip-old-msgs")

		var ts *types.TipSet
		if tss := cctx.String("tipset"); tss != "" {
			cids, err := lcli.ParseTipSetString(tss)
			if err != nil {
				return xerrors.Errorf("failed to parse tipset (%q): %w", tss, err)
			}

			tsk := types.NewTipSetKey(cids...)

			selts, err := cs.LoadTipSet(tsk)
			if err != nil {
				return xerrors.Errorf("loading tipset: %w", err)
			}
			ts = selts
		} else {
			ts = cs.GetHeaviestTipSet()
		}

		if fullstate {
			nroots = ts.Height() + 1
		}

		if err := cs.Export(ctx, ts, nroots, skipoldmsgs, fi); err != nil {
			return xerrors.Errorf("export failed: %w", err)
		}

		return nil
	},
}
