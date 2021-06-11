package blockstore

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

var _ Blockstore = (*noopstore)(nil)

type noopstore struct {
	bs Blockstore
}

func NewNoopStore(bs Blockstore) Blockstore {
	return &noopstore{bs: bs}
}

func (b *noopstore) Has(cid cid.Cid) (bool, error) {
	return b.bs.Has(cid)
}

func (b *noopstore) HashOnRead(hor bool) {
	b.bs.HashOnRead(hor)
}

func (b *noopstore) Get(cid cid.Cid) (blocks.Block, error) {
	return b.bs.Get(cid)
}

func (b *noopstore) GetSize(cid cid.Cid) (int, error) {
	return b.bs.GetSize(cid)
}

func (b *noopstore) View(cid cid.Cid, f func([]byte) error) error {
	return b.bs.View(cid, f)
}

func (b *noopstore) Put(blk blocks.Block) error {
	return nil
}

func (b *noopstore) PutMany(blks []blocks.Block) error {
	return nil
}

func (b *noopstore) DeleteBlock(cid cid.Cid) error {
	return nil
}

func (b *noopstore) DeleteMany(cids []cid.Cid) error {
	return nil
}

func (b *noopstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.bs.AllKeysChan(ctx)
}

func (b *noopstore) Close() error {
	if c, ok := b.bs.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
