package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
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

		// Snapshots are distributed zstd-compressed; read them in-stream rather
		// than requiring the caller to decompress first.
		bufr := bufio.NewReaderSize(f, 1<<20)
		var src io.Reader = bufr
		if hdr, err := bufr.Peek(4); err == nil && string(hdr[1:]) == "\xB5\x2F\xFD" {
			zr, err := zstd.NewReader(bufr)
			if err != nil {
				return xerrors.Errorf("instantiating zstd reader: %w", err)
			}
			defer zr.Close()
			src = zr
		}

		// BlockReader picks the CAR version from the pragma, so both v1 and v2
		// are accepted.
		cr, err := carv2.NewBlockReader(src)
		if err != nil {
			return err
		}

		// An FRC-0108 v2 snapshot leads with a metadata block, optionally
		// followed by an F3 section. That section is far larger than the
		// maximum block size, so it has to be streamed past rather than read
		// as a block; otherwise the read fails on the section length.
		if len(cr.Roots) == store.V2SnapshotRootCount {
			blk, err := cr.Next()
			if err != nil {
				return xerrors.Errorf("reading snapshot metadata block: %w", err)
			}
			var metadata store.SnapshotMetadata
			if err := metadata.UnmarshalCBOR(bytes.NewReader(blk.RawData())); err != nil {
				// Not metadata after all; it belongs in the blockstore.
				if err := bs.Put(ctx, blk); err != nil {
					return xerrors.Errorf("put %s: %w", blk.Cid(), err)
				}
			} else if metadata.F3Data != nil {
				_, f3Reader, _, err := cr.NextReader()
				if err != nil {
					return xerrors.Errorf("reading F3 section: %w", err)
				}
				if _, err := io.Copy(io.Discard, f3Reader); err != nil {
					return xerrors.Errorf("skipping F3 section: %w", err)
				}
			}
		}

		for {
			blk, err := cr.Next()
			switch err {
			case io.EOF:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Println()
				return nil
			default:
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Println()
				return err
			case nil:
				fmt.Printf("\r%s", blk.Cid())
				if err := bs.Put(ctx, blk); err != nil {
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

		if err := bs.Put(ctx, blk); err != nil {
			return err
		}

		return nil

	},
}
