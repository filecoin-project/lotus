package blockstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	blockstore "github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// ErrIdentityCid is returned for reads of identity CIDs, which can't reference stored blocks.
// It is distinct from ipld.ErrNotFound so callers don't look for the block on the network.
var ErrIdentityCid = errors.New("identity CID rejected by blockstore")

// IsIdentityCid reports whether c uses the identity multihash.
func IsIdentityCid(c cid.Cid) bool {
	return c.Defined() && c.Prefix().MhType == mh.IDENTITY
}

// IdentityCidError returns ErrIdentityCid wrapped with c.
func IdentityCidError(c cid.Cid) error {
	return fmt.Errorf("%w: %s", ErrIdentityCid, c)
}

// RejectIdentityCids wraps a blockstore so that reads of identity CIDs fail with
// ErrIdentityCid. Writes and deletes of identity CIDs are dropped: genesis CARs and older
// snapshots carry the pre-nv16 actor code CIDs as blocks and must still import.
func RejectIdentityCids(bstore blockstore.Blockstore) Blockstore {
	if rs, ok := bstore.(*idRejectStore); ok {
		// already wrapped
		return rs
	}
	return &idRejectStore{Adapt(bstore)}
}

type idRejectStore struct {
	Blockstore
}

var _ Blockstore = (*idRejectStore)(nil)

func (b *idRejectStore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if IsIdentityCid(c) {
		return false, IdentityCidError(c)
	}
	return b.Blockstore.Has(ctx, c)
}

func (b *idRejectStore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if IsIdentityCid(c) {
		return nil, IdentityCidError(c)
	}
	return b.Blockstore.Get(ctx, c)
}

func (b *idRejectStore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if IsIdentityCid(c) {
		return 0, IdentityCidError(c)
	}
	return b.Blockstore.GetSize(ctx, c)
}

func (b *idRejectStore) View(ctx context.Context, c cid.Cid, cb func([]byte) error) error {
	if IsIdentityCid(c) {
		return IdentityCidError(c)
	}
	return b.Blockstore.View(ctx, c, cb)
}

func (b *idRejectStore) Put(ctx context.Context, blk blocks.Block) error {
	if IsIdentityCid(blk.Cid()) {
		return nil
	}
	return b.Blockstore.Put(ctx, blk)
}

func (b *idRejectStore) PutMany(ctx context.Context, blks []blocks.Block) error {
	blks = WithoutIdentityBlocks(blks)
	if len(blks) == 0 {
		return nil
	}
	return b.Blockstore.PutMany(ctx, blks)
}

func (b *idRejectStore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	if IsIdentityCid(c) {
		return nil
	}
	return b.Blockstore.DeleteBlock(ctx, c)
}

func (b *idRejectStore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	cids = withoutIdentity(cids, func(c cid.Cid) cid.Cid { return c })
	if len(cids) == 0 {
		return nil
	}
	return b.Blockstore.DeleteMany(ctx, cids)
}

// WithoutIdentityBlocks returns blks minus any identity-CID blocks, reusing blks when
// there are none.
func WithoutIdentityBlocks(blks []blocks.Block) []blocks.Block {
	return withoutIdentity(blks, blocks.Block.Cid)
}

func withoutIdentity[T any](items []T, cidOf func(T) cid.Cid) []T {
	for i, item := range items {
		if !IsIdentityCid(cidOf(item)) {
			continue
		}
		out := append(make([]T, 0, len(items)-1), items[:i]...)
		for _, item := range items[i+1:] {
			if !IsIdentityCid(cidOf(item)) {
				out = append(out, item)
			}
		}
		return out
	}
	return items
}

// The remaining methods are optional blockstore traits; forward them so that wrapping
// does not hide a capability of the underlying store.

func (b *idRejectStore) ForEachKey(f func(cid.Cid) error) error {
	iterBstore, ok := b.Blockstore.(BlockstoreIterator)
	if !ok {
		return fmt.Errorf("underlying blockstore (type %T) doesn't support fast iteration", b.Blockstore)
	}
	return iterBstore.ForEachKey(f)
}

func (b *idRejectStore) Close() error {
	if c, ok := b.Blockstore.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (b *idRejectStore) CollectGarbage(ctx context.Context, options ...BlockstoreGCOption) error {
	if bs, ok := b.Blockstore.(BlockstoreGC); ok {
		return bs.CollectGarbage(ctx, options...)
	}
	return errors.New("not supported")
}

func (b *idRejectStore) GCOnce(ctx context.Context, options ...BlockstoreGCOption) error {
	if bs, ok := b.Blockstore.(BlockstoreGCOnce); ok {
		return bs.GCOnce(ctx, options...)
	}
	return errors.New("not supported")
}
