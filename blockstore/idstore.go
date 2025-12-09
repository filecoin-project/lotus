package blockstore

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

var _ Blockstore = (*idstore)(nil)

type idstore struct {
	bs Blockstore
}

func NewIDStore(bs Blockstore) Blockstore {
	return &idstore{bs: bs}
}

func decodeCid(cid cid.Cid) (inline bool, data []byte, err error) {
	if cid.Prefix().MhType != mh.IDENTITY {
		return false, nil, nil
	}

	dmh, err := mh.Decode(cid.Hash())
	if err != nil {
		return false, nil, err
	}

	if dmh.Code == mh.IDENTITY {
		return true, dmh.Digest, nil
	}

	return false, nil, err
}

func (b *idstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	inline, _, err := decodeCid(cid)
	if err != nil {
		return false, xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return true, nil
	}

	return b.bs.Has(ctx, cid)
}

func (b *idstore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	inline, data, err := decodeCid(cid)
	if err != nil {
		return nil, xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return blocks.NewBlockWithCid(data, cid)
	}

	return b.bs.Get(ctx, cid)
}

func (b *idstore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	inline, data, err := decodeCid(cid)
	if err != nil {
		return 0, xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return len(data), err
	}

	return b.bs.GetSize(ctx, cid)
}

func (b *idstore) View(ctx context.Context, cid cid.Cid, cb func([]byte) error) error {
	inline, data, err := decodeCid(cid)
	if err != nil {
		return xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return cb(data)
	}

	return b.bs.View(ctx, cid, cb)
}

func (b *idstore) Put(ctx context.Context, blk blocks.Block) error {
	inline, _, err := decodeCid(blk.Cid())
	if err != nil {
		return xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return nil
	}

	return b.bs.Put(ctx, blk)
}

func (b *idstore) ForEachKey(f func(cid.Cid) error) error {
	iterBstore, ok := b.bs.(BlockstoreIterator)
	if !ok {
		return xerrors.Errorf("underlying blockstore (type %T) doesn't support fast iteration", b.bs)
	}
	return iterBstore.ForEachKey(f)
}

func (b *idstore) PutMany(ctx context.Context, blks []blocks.Block) error {
	toPut := make([]blocks.Block, 0, len(blks))
	for _, blk := range blks {
		inline, _, err := decodeCid(blk.Cid())
		if err != nil {
			return xerrors.Errorf("error decoding Cid: %w", err)
		}

		if inline {
			continue
		}
		toPut = append(toPut, blk)
	}

	if len(toPut) > 0 {
		return b.bs.PutMany(ctx, toPut)
	}

	return nil
}

func (b *idstore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	inline, _, err := decodeCid(cid)
	if err != nil {
		return xerrors.Errorf("error decoding Cid: %w", err)
	}

	if inline {
		return nil
	}

	return b.bs.DeleteBlock(ctx, cid)
}

func (b *idstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	toDelete := make([]cid.Cid, 0, len(cids))
	for _, cid := range cids {
		inline, _, err := decodeCid(cid)
		if err != nil {
			return xerrors.Errorf("error decoding Cid: %w", err)
		}

		if inline {
			continue
		}
		toDelete = append(toDelete, cid)
	}

	if len(toDelete) > 0 {
		return b.bs.DeleteMany(ctx, toDelete)
	}

	return nil
}

func (b *idstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.bs.AllKeysChan(ctx)
}

func (b *idstore) Close() error {
	if c, ok := b.bs.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (b *idstore) Flush(ctx context.Context) error {
	return b.bs.Flush(ctx)
}

func (b *idstore) CollectGarbage(ctx context.Context, options ...BlockstoreGCOption) error {
	if bs, ok := b.bs.(BlockstoreGC); ok {
		return bs.CollectGarbage(ctx, options...)
	}
	return xerrors.Errorf("not supported")
}

func (b *idstore) GCOnce(ctx context.Context, options ...BlockstoreGCOption) error {
	if bs, ok := b.bs.(BlockstoreGCOnce); ok {
		return bs.GCOnce(ctx, options...)
	}
	return xerrors.Errorf("not supported")
}
