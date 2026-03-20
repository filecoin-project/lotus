package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

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

		ctx := context.TODO()

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

		bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return err
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		// Detect zstd compression by magic bytes
		bufr := bufio.NewReaderSize(f, 4<<20) // 4 MiB buffer
		header, err := bufr.Peek(4)
		if err != nil {
			return xerrors.Errorf("peek header: %w", err)
		}

		var rd io.Reader = bufr
		if len(header) >= 4 && header[0] == 0x28 && header[1] == 0xB5 && header[2] == 0x2F && header[3] == 0xFD {
			zr, err := zstd.NewReader(bufr)
			if err != nil {
				return xerrors.Errorf("creating zstd reader: %w", err)
			}
			defer zr.Close()
			rd = zr
			fmt.Println("Detected zstd compression, decompressing on the fly")
		}

		cr, err := car.NewCarReader(rd)
		if err != nil {
			return err
		}

		var count int
		for {
			blk, err := cr.Next()
			switch err {
			case io.EOF:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Printf("\rImported %d blocks\n", count)
				return nil
			default:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Println()
				return err
			case nil:
				count++
				if count%1000 == 0 {
					fmt.Printf("\rImported %dk blocks", count/1000)
				}
				if err := bs.Put(context.Background(), blk); err != nil {
					if err := f.Close(); err != nil {
						return err
					}
					return xerrors.Errorf("put %s: %w", blk.Cid(), err)
				}
			}
		}
	},
}

var importObjectCmd = &cli.Command{
	Name:  "import-obj",
	Usage: "import a raw ipld object into your datastore",
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		ctx := context.TODO()

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

		bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
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

		c, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		data, err := hex.DecodeString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		blk, err := block.NewBlockWithCid(data, c)
		if err != nil {
			return err
		}

		if err := bs.Put(context.Background(), blk); err != nil {
			return err
		}

		return nil

	},
}
