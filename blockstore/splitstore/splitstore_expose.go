package splitstore

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

type exposedSplitStore struct {
	s *SplitStore
}

var _ bstore.Blockstore = (*exposedSplitStore)(nil)

func (s *SplitStore) Expose() bstore.Blockstore {
	return &exposedSplitStore{s: s}
}

func (es *exposedSplitStore) DeleteBlock(_ cid.Cid) error {
	return errors.New("DeleteBlock: operation not supported")
}

func (es *exposedSplitStore) DeleteMany(_ []cid.Cid) error {
	return errors.New("DeleteMany: operation not supported")
}

func (es *exposedSplitStore) Has(c cid.Cid) (bool, error) {
	if isIdentiyCid(c) {
		return true, nil
	}

	has, err := es.s.hot.Has(c)
	if has || err != nil {
		return has, err
	}

	return es.s.cold.Has(c)
}

func (es *exposedSplitStore) Get(c cid.Cid) (blocks.Block, error) {
	if isIdentiyCid(c) {
		data, err := decodeIdentityCid(c)
		if err != nil {
			return nil, err
		}

		return blocks.NewBlockWithCid(data, c)
	}

	blk, err := es.s.hot.Get(c)
	switch err {
	case bstore.ErrNotFound:
		return es.s.cold.Get(c)
	default:
		return blk, err
	}
}

func (es *exposedSplitStore) GetSize(c cid.Cid) (int, error) {
	if isIdentiyCid(c) {
		data, err := decodeIdentityCid(c)
		if err != nil {
			return 0, err
		}

		return len(data), nil
	}

	size, err := es.s.hot.GetSize(c)
	switch err {
	case bstore.ErrNotFound:
		return es.s.cold.GetSize(c)
	default:
		return size, err
	}
}

func (es *exposedSplitStore) Put(blk blocks.Block) error {
	return es.s.Put(blk)
}

func (es *exposedSplitStore) PutMany(blks []blocks.Block) error {
	return es.s.PutMany(blks)
}

func (es *exposedSplitStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return es.s.AllKeysChan(ctx)
}

func (es *exposedSplitStore) HashOnRead(enabled bool) {}

func (es *exposedSplitStore) View(c cid.Cid, f func([]byte) error) error {
	if isIdentiyCid(c) {
		data, err := decodeIdentityCid(c)
		if err != nil {
			return err
		}

		return f(data)
	}

	err := es.s.hot.View(c, f)
	switch err {
	case bstore.ErrNotFound:
		return es.s.cold.View(c, f)

	default:
		return err
	}
}
