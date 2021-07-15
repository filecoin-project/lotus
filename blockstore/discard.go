package blockstore

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

var _ Blockstore = (*discardstore)(nil)

type discardstore struct {
	bs Blockstore
}

func NewDiscardStore(bs Blockstore) Blockstore {
	return &discardstore{bs: bs}
}

func (b *discardstore) Has(cid cid.Cid) (bool, error) {
	return b.bs.Has(cid)
}

func (b *discardstore) HashOnRead(hor bool) {
	b.bs.HashOnRead(hor)
}

func (b *discardstore) Get(cid cid.Cid) (blocks.Block, error) {
	return b.bs.Get(cid)
}

func (b *discardstore) GetSize(cid cid.Cid) (int, error) {
	return b.bs.GetSize(cid)
}

func (b *discardstore) View(cid cid.Cid, f func([]byte) error) error {
	return b.bs.View(cid, f)
}

func (b *discardstore) Put(blk blocks.Block) error {
	return nil
}

func (b *discardstore) PutMany(blks []blocks.Block) error {
	return nil
}

func (b *discardstore) DeleteBlock(cid cid.Cid) error {
	return nil
}

func (b *discardstore) DeleteMany(cids []cid.Cid) error {
	return nil
}

func (b *discardstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.bs.AllKeysChan(ctx)
}

func (b *discardstore) Close() error {
	if c, ok := b.bs.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
