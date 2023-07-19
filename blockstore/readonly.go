package blockstore

import (
	"context"
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type ReadonlyBlockstore struct {
	base Blockstore
}

func WithReadonly(base Blockstore) *ReadonlyBlockstore {
	bs := &ReadonlyBlockstore{
		base: base,
	}
	return bs
}

func ReadonlyError(description string) error {
	return fmt.Errorf("Write protected access attempted on Readonly Blockstore from method %s", description)
}

func (bs *ReadonlyBlockstore) Flush(ctx context.Context) error {
	return ReadonlyError("Flush")
}

func (bs *ReadonlyBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return ReadonlyError("DeleteBlock")
}

func (bs *ReadonlyBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	return ReadonlyError("DeleteMany")
}
func (bs *ReadonlyBlockstore) Put(ctx context.Context, blk block.Block) error {
	return ReadonlyError("Put")
}
func (bs *ReadonlyBlockstore) PutMany(ctx context.Context, blks []block.Block) error {
	return ReadonlyError("PutMany")
}

func (bs *ReadonlyBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.base.AllKeysChan(ctx)
}

func (bs *ReadonlyBlockstore) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	return bs.base.View(ctx, c, callback)
}
func (bs *ReadonlyBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	return bs.base.Get(ctx, c)
}
func (bs *ReadonlyBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	return bs.base.GetSize(ctx, c)
}
func (bs *ReadonlyBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	return bs.base.Has(ctx, c)
}
func (bs *ReadonlyBlockstore) HashOnRead(hor bool) {
	log.Warnf("called HashOnRead on Readonly blockstore; function not supported; ignoring")
}
