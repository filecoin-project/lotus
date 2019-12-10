package retrievalimpl

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	pb "github.com/ipfs/go-unixfs/pb"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
)

type BlockVerifier interface {
	Verify(context.Context, blocks.Block) (internal bool, err error)
}

type OptimisticVerifier struct {
}

func (o *OptimisticVerifier) Verify(context.Context, blocks.Block) (bool, error) {
	// It's probably fine
	return false, nil
}

type UnixFs0Verifier struct {
	Root    cid.Cid
	rootBlk blocks.Block

	expect int
	seen   int

	sub *UnixFs0Verifier
}

func (b *UnixFs0Verifier) verify(ctx context.Context, blk blocks.Block) (last bool, internal bool, err error) {
	if b.sub != nil {
		// TODO: check links here (iff b.sub.sub == nil)

		subLast, internal, err := b.sub.verify(ctx, blk)
		if err != nil {
			return false, false, err
		}
		if subLast {
			b.sub = nil
			b.seen++
		}

		return b.seen == b.expect, internal, nil
	}

	if b.seen >= b.expect { // this is probably impossible
		return false, false, xerrors.New("unixfs verifier: too many nodes in level")
	}

	links, err := b.checkInternal(blk)
	if err != nil {
		return false, false, err
	}

	if links > 0 { // TODO: check if all links are intermediate (or all aren't)
		if links > build.UnixfsLinksPerLevel {
			return false, false, xerrors.New("unixfs verifier: too many links in intermediate node")
		}

		if b.seen+1 == b.expect && links != build.UnixfsLinksPerLevel {
			return false, false, xerrors.New("unixfs verifier: too few nodes in level")
		}

		b.sub = &UnixFs0Verifier{
			Root:    blk.Cid(),
			rootBlk: blk,
			expect:  links,
		}

		// don't mark as seen yet
		return false, true, nil
	}

	b.seen++
	return b.seen == b.expect, false, nil
}

func (b *UnixFs0Verifier) checkInternal(blk blocks.Block) (int, error) {
	nd, err := ipld.Decode(blk)
	if err != nil {
		log.Warnf("IPLD Decode failed: %s", err)
		return 0, err
	}

	// TODO: check size
	switch nd := nd.(type) {
	case *merkledag.ProtoNode:
		fsn, err := unixfs.FSNodeFromBytes(nd.Data())
		if err != nil {
			log.Warnf("unixfs.FSNodeFromBytes failed: %s", err)
			return 0, err
		}
		if fsn.Type() != pb.Data_File {
			return 0, xerrors.New("internal nodes must be a file")
		}
		if len(fsn.Data()) > 0 {
			return 0, xerrors.New("internal node with data")
		}
		if len(nd.Links()) == 0 {
			return 0, xerrors.New("internal node with no links")
		}
		return len(nd.Links()), nil

	case *merkledag.RawNode:
		return 0, nil
	default:
		return 0, xerrors.New("verifier: unknown node type")
	}
}

func (b *UnixFs0Verifier) Verify(ctx context.Context, blk blocks.Block) (bool, error) {
	// root is special
	if b.rootBlk == nil {
		if !b.Root.Equals(blk.Cid()) {
			return false, xerrors.Errorf("unixfs verifier: root block CID didn't match: valid %s, got %s", b.Root, blk.Cid())
		}
		b.rootBlk = blk
		links, err := b.checkInternal(blk)
		if err != nil {
			return false, err
		}

		b.expect = links
		return links != 0, nil
	}

	_, internal, err := b.verify(ctx, blk)
	return internal, err
}

var _ BlockVerifier = &OptimisticVerifier{}
var _ BlockVerifier = &UnixFs0Verifier{}
