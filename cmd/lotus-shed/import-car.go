package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
)

var importCarCmd = &cli.Command{
	Name:        "import-car",
	Description: "Import a car file into node chain blockstore",
	Action: func(cctx *cli.Context) error {
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

		cf := cctx.Args().Get(0)
		f, err := os.OpenFile(cf, os.O_RDONLY, 0664)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}

		ds, err := lr.Datastore("/chain")
		if err != nil {
			return err
		}

		bs := blockstore.NewBlockstore(ds)

		cr, err := car.NewCarReader(f)
		if err != nil {
			return err
		}

		for {
			blk, err := cr.Next()
			switch err {
			case io.EOF:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Println()
				return ds.Close()
			default:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Println()
				return err
			case nil:
				fmt.Printf("\r%s", blk.Cid())
				if err := bs.Put(blk); err != nil {
					if err := f.Close(); err != nil {
						return err
					}
					return xerrors.Errorf("put %s: %w", blk.Cid(), err)
				}
			}
		}
	},
}
