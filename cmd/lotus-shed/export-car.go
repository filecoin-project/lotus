package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

func carWalkFunc(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}
	return out, nil
}

var exportCarCmd = &cli.Command{
	Name:        "export-car",
	Description: "Export a car from repo (requires node to be offline)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return lcli.ShowHelp(cctx, fmt.Errorf("must specify file name and object"))
		}

		outfile := cctx.Args().First()
		var roots []cid.Cid
		for _, arg := range cctx.Args().Tail() {
			c, err := cid.Decode(arg)
			if err != nil {
				return err
			}
			roots = append(roots, c)
		}

		ctx := lcli.ReqContext(cctx)

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

		fi, err := os.Create(outfile)
		if err != nil {
			return xerrors.Errorf("opening the output file: %w", err)
		}

		defer fi.Close() //nolint:errcheck

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

		dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
		err = car.WriteCarWithWalker(ctx, dag, roots, fi, carWalkFunc)
		if err != nil {
			return err
		}
		return nil
	},
}
