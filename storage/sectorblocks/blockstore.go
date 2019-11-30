package sectorblocks

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

type SectorBlockStore struct {
	intermediate blockstore.Blockstore
	sectorBlocks *SectorBlocks

	approveUnseal func() error
}

func (s *SectorBlockStore) DeleteBlock(cid.Cid) error {
	panic("not supported")
}
func (s *SectorBlockStore) GetSize(cid.Cid) (int, error) {
	panic("not supported")
}

func (s *SectorBlockStore) Put(blocks.Block) error {
	panic("not supported")
}

func (s *SectorBlockStore) PutMany([]blocks.Block) error {
	panic("not supported")
}

func (s *SectorBlockStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	panic("not supported")
}

func (s *SectorBlockStore) HashOnRead(enabled bool) {
	panic("not supported")
}

func (s *SectorBlockStore) Has(c cid.Cid) (bool, error) {
	has, err := s.intermediate.Has(c)
	if err != nil {
		return false, err
	}
	if has {
		return true, nil
	}

	return s.sectorBlocks.Has(c)
}

func (s *SectorBlockStore) Get(c cid.Cid) (blocks.Block, error) {
	val, err := s.intermediate.Get(c)
	if err == nil {
		return val, nil
	}
	if err != blockstore.ErrNotFound {
		return nil, err
	}

	refs, err := s.sectorBlocks.GetRefs(c)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, blockstore.ErrNotFound
	}

	data, err := s.sectorBlocks.unsealed.getRef(context.TODO(), refs, s.approveUnseal)
	if err != nil {
		return nil, err
	}

	return blocks.NewBlockWithCid(data, c)
}

var _ blockstore.Blockstore = &SectorBlockStore{}
