package splitstore

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

type exposedSplitStore struct {
	s *SplitStore
}

var _ bstore.Blockstore = (*exposedSplitStore)(nil)

func (s *SplitStore) Expose() bstore.Blockstore {
	return &exposedSplitStore{s: s}
}

func (es *exposedSplitStore) DeleteBlock(_ context.Context, _ cid.Cid) error {
	return errors.New("DeleteBlock: operation not supported")
}

func (es *exposedSplitStore) DeleteMany(_ context.Context, _ []cid.Cid) error {
	return errors.New("DeleteMany: operation not supported")
}

func (es *exposedSplitStore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if bstore.IsIdentityCid(c) {
		return false, bstore.IdentityCidError(c)
	}

	has, err := es.s.hot.Has(ctx, c)
	if has || err != nil {
		return has, err
	}

	return es.s.cold.Has(ctx, c)
}

func (es *exposedSplitStore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {

	if bstore.IsIdentityCid(c) {
		return nil, bstore.IdentityCidError(c)
	}

	blk, err := es.s.hot.Get(ctx, c)
	if ipld.IsNotFound(err) {
		return es.s.cold.Get(ctx, c)
	}
	return blk, err
}

func (es *exposedSplitStore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if bstore.IsIdentityCid(c) {
		return 0, bstore.IdentityCidError(c)
	}

	size, err := es.s.hot.GetSize(ctx, c)
	if ipld.IsNotFound(err) {
		return es.s.cold.GetSize(ctx, c)
	}
	return size, err
}

func (es *exposedSplitStore) Flush(ctx context.Context) error {
	return es.s.Flush(ctx)
}

func (es *exposedSplitStore) Put(ctx context.Context, blk blocks.Block) error {
	return es.s.Put(ctx, blk)
}

func (es *exposedSplitStore) PutMany(ctx context.Context, blks []blocks.Block) error {
	return es.s.PutMany(ctx, blks)
}

func (es *exposedSplitStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return es.s.AllKeysChan(ctx)
}

func (es *exposedSplitStore) View(ctx context.Context, c cid.Cid, f func([]byte) error) error {
	if bstore.IsIdentityCid(c) {
		return bstore.IdentityCidError(c)
	}

	err := es.s.hot.View(ctx, c, f)
	if ipld.IsNotFound(err) {
		return es.s.cold.View(ctx, c, f)
	}

	return err
}
