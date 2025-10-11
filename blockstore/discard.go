package blockstore

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

var _ Blockstore = (*discardstore)(nil)

type discardstore struct {
	bs Blockstore
}

func NewDiscardStore(bs Blockstore) Blockstore {
	return &discardstore{bs: bs}
}

func (b *discardstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	return b.bs.Has(ctx, cid)
}

func (b *discardstore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	return b.bs.Get(ctx, cid)
}

func (b *discardstore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	return b.bs.GetSize(ctx, cid)
}

func (b *discardstore) View(ctx context.Context, cid cid.Cid, f func([]byte) error) error {
	return b.bs.View(ctx, cid, f)
}

func (b *discardstore) Flush(ctx context.Context) error {
	return nil
}

func (b *discardstore) Put(ctx context.Context, blk blocks.Block) error {
	return nil
}

func (b *discardstore) PutMany(ctx context.Context, blks []blocks.Block) error {
	return nil
}

func (b *discardstore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	return nil
}

func (b *discardstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
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
