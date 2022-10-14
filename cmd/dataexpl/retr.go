package main

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/markets/utils"
)

func findRoot(ctx context.Context, root cid.Cid, selspec builder.SelectorSpec, ds format.DAGService) (cid.Cid, map[string]struct{}, error) {
	rsn := selspec.Node()

	links := map[string]struct{}{}

	var newRoot cid.Cid
	var errHalt = errors.New("halt walk")
	if err := utils.TraverseDag(
		ctx,
		ds,
		root,
		rsn,
		nil,
		func(p traversal.Progress, n ipld.Node, r traversal.VisitReason) error {

			links[p.LastBlock.Path.String()] = struct{}{}

			if r == traversal.VisitReason_SelectionMatch {
				if p.LastBlock.Path.String() != p.Path.String() {
					return xerrors.Errorf("unsupported selection path '%s' does not correspond to a block boundary (a.k.a. CID link)", p.Path.String())
				}

				if p.LastBlock.Link == nil {
					// this is likely the root node that we've matched here
					newRoot = root
					return errHalt
				}

				cidLnk, castOK := p.LastBlock.Link.(cidlink.Link)
				if !castOK {
					return xerrors.Errorf("cidlink cast unexpectedly failed on '%s'", p.LastBlock.Link)
				}

				newRoot = cidLnk.Cid

				return errHalt
			}
			return nil
		},
	); err != nil && err != errHalt {
		return cid.Undef, nil, xerrors.Errorf("error while locating partial retrieval sub-root: %w", err)
	}

	if newRoot == cid.Undef {
		return cid.Undef, nil, xerrors.Errorf("path selection does not match a node within %s", root)
	}
	return newRoot, links, nil
}
