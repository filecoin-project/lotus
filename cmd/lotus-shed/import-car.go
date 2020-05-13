package main

import (
	"fmt"
	"io"
	"os"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

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
		defer lr.Close()

		cf := cctx.Args().Get(0)
		f, err := os.OpenFile(cf, os.O_RDONLY, 0664)
		if err != nil {
			return xerrors.Errorf("opening the car file: %w", err)
		}

		ds, err := lr.Datastore("/blocks")
		if err != nil {
			return err
		}

		bs := blockstore.NewBlockstore(ds)
		bs = blockstore.NewIdStore(bs)

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
				fmt.Println()
				return err
			case nil:
				fmt.Printf("\r%s", blk.Cid())
				if err := bs.Put(blk); err != nil {
					return xerrors.Errorf("put %s: %w", blk.Cid(), err)
				}
			}
		}
	},
}
