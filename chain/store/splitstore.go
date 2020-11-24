package store

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	bstore2 "github.com/filecoin-project/lotus/lib/blockstore"
)

type SplitStore struct {
	baseEpoch abi.ChainEpoch
	curTs     *types.TipSet

	cs *ChainStore

	hot  bstore2.Blockstore
	cold bstore2.Blockstore

	snoop TrackingStore
	sweep TrackingStore
}

type TrackingStore interface {
	Put(cid.Cid, abi.ChainEpoch) error
	PutBatch([]cid.Cid, abi.ChainEpoch) error
	Get(cid.Cid) (abi.ChainEpoch, error)
	Delete(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Keys() (<-chan cid.Cid, error)
}

var _ bstore2.Blockstore = (*SplitStore)(nil)

// Blockstore interface
func (s *SplitStore) DeleteBlock(cid cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteBlock not implemented on SplitStore; don't do this Luke!")
}

func (s *SplitStore) Has(cid cid.Cid) (bool, error) {
	has, err := s.hot.Has(cid)

	if err != nil {
		return false, err
	}

	if has {
		return true, nil
	}

	return s.cold.Has(cid)
}

func (s *SplitStore) Get(cid cid.Cid) (blocks.Block, error) {
	blk, err := s.hot.Get(cid)

	switch err {
	case nil:
		return blk, nil

	case bstore.ErrNotFound:
		return s.cold.Get(cid)

	default:
		return nil, err
	}
}

func (s *SplitStore) GetSize(cid cid.Cid) (int, error) {
	size, err := s.hot.GetSize(cid)

	switch err {
	case nil:
		return size, nil

	case bstore.ErrNotFound:
		return s.cold.GetSize(cid)

	default:
		return 0, err
	}
}

func (s *SplitStore) Put(blk blocks.Block) error {
	epoch := s.curTs.Height()
	err := s.snoop.Put(blk.Cid(), epoch)
	if err != nil {
		return err
	}

	return s.hot.Put(blk)
}

func (s *SplitStore) PutMany(blks []blocks.Block) error {
	err := s.hot.PutMany(blks)
	if err != nil {
		return err
	}

	epoch := s.curTs.Height()

	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	return s.snoop.PutBatch(batch, epoch)
}

func (s *SplitStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	ctx, cancel := context.WithCancel(ctx)

	chHot, err := s.hot.AllKeysChan(ctx)
	if err != nil {
		return nil, err
	}

	chCold, err := s.cold.AllKeysChan(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	ch := make(chan cid.Cid)
	go func() {
		defer cancel()

		for _, in := range []<-chan cid.Cid{chHot, chCold} {
			for cid := range in {
				select {
				case ch <- cid:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (s *SplitStore) HashOnRead(enabled bool) {
	s.hot.HashOnRead(enabled)
	s.cold.HashOnRead(enabled)
}

func (s *SplitStore) View(cid cid.Cid, cb func([]byte) error) error {
	err := s.hot.View(cid, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(cid, cb)

	default:
		return err
	}
}

// Compaction/GC Algorithm
func (s *SplitStore) compact() {
	// Phase 1: mark all reachable CIDs with the current epoch
	curTs := s.curTs
	epoch := curTs.Height()
	err := s.cs.WalkSnapshot(context.Background(), curTs, epoch-s.baseEpoch, false, false,
		func(cid cid.Cid) error {
			return s.sweep.Put(cid, epoch)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	// Phase 2: sweep cold objects, moving reachable ones to the coldstore and deleting the others
	coldEpoch := s.baseEpoch + build.Finality

	ch, err := s.snoop.Keys()
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	for cid := range ch {
		wrEpoch, err := s.snoop.Get(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		// is the object stil hot?
		if wrEpoch >= coldEpoch {
			// yes, just clear the mark and continue
			err := s.sweep.Delete(cid)
			if err != nil {
				// TODO do something better here
				panic(err)
			}
			continue
		}

		// the object is cold -- check whether it is reachable
		mark, err := s.sweep.Has(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		if mark {
			// the object is reachable, move it to the cold store and delete the mark
			blk, err := s.hot.Get(cid)
			if err != nil {
				// TODO do something better here
				panic(err)
			}

			err = s.cold.Put(blk)
			if err != nil {
				// TODO do something better here
				panic(err)
			}

			err = s.sweep.Delete(cid)
			if err != nil {
				// TODO do something better here
				panic(err)
			}
		}

		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		// remove the snoop tracking
		err = s.snoop.Delete(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}
	}

	// clear all remaining marks for cold objects that may have been reachable
	ch, err = s.sweep.Keys()
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	for cid := range ch {
		err = s.sweep.Delete(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}
	}

	// TODO persist base epoch to metadata ds
	s.baseEpoch = coldEpoch
}
