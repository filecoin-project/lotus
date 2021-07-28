package splitstore

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

type reifyingSplitStore struct {
	s *SplitStore
}

var _ bstore.Blockstore = (*reifyingSplitStore)(nil)
var _ bstore.BlockstoreHotView = (*SplitStore)(nil)

func (s *SplitStore) HotView() bstore.Blockstore {
	return &reifyingSplitStore{s: s}
}

func (rs *reifyingSplitStore) DeleteBlock(_ cid.Cid) error {
	return errors.New("DeleteBlock: operation not supported")
}

func (rs *reifyingSplitStore) DeleteMany(_ []cid.Cid) error {
	return errors.New("DeleteMany: operation not supported")
}

func (rs *reifyingSplitStore) Has(c cid.Cid) (bool, error) {
	return rs.s.iHas(c, true)
}

func (rs *reifyingSplitStore) Get(c cid.Cid) (blocks.Block, error) {
	return rs.s.iGet(c, true)
}

func (rs *reifyingSplitStore) GetSize(c cid.Cid) (int, error) {
	return rs.s.iGetSize(c, true)
}

func (rs *reifyingSplitStore) Put(blk blocks.Block) error {
	return rs.s.Put(blk)
}

func (rs *reifyingSplitStore) PutMany(blks []blocks.Block) error {
	return rs.s.PutMany(blks)
}

func (rs *reifyingSplitStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return rs.s.AllKeysChan(ctx)
}

func (rs *reifyingSplitStore) HashOnRead(enabled bool) {}

func (rs *reifyingSplitStore) View(c cid.Cid, f func([]byte) error) error {
	return rs.s.iView(c, true, f)
}
