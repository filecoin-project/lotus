package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type unionBlockstore []Blockstore

// Union returns an unioned blockstore.
//
// * Reads return from the first blockstore that has the value, querying in the
//   supplied order.
// * Writes (puts and deltes) are broadcast to all stores.
//
func Union(stores ...Blockstore) Blockstore {
	return unionBlockstore(stores)
}

func (m unionBlockstore) Has(cid cid.Cid) (has bool, err error) {
	for _, bs := range m {
		if has, err = bs.Has(cid); has || err != nil {
			break
		}
	}
	return has, err
}

func (m unionBlockstore) Get(cid cid.Cid) (blk blocks.Block, err error) {
	for _, bs := range m {
		if blk, err = bs.Get(cid); err == nil || err != ErrNotFound {
			break
		}
	}
	return blk, err
}

func (m unionBlockstore) View(cid cid.Cid, callback func([]byte) error) (err error) {
	for _, bs := range m {
		if err = bs.View(cid, callback); err == nil || err != ErrNotFound {
			break
		}
	}
	return err
}

func (m unionBlockstore) GetSize(cid cid.Cid) (size int, err error) {
	for _, bs := range m {
		if size, err = bs.GetSize(cid); err == nil || err != ErrNotFound {
			break
		}
	}
	return size, err
}

func (m unionBlockstore) Put(block blocks.Block) (err error) {
	for _, bs := range m {
		if err = bs.Put(block); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) PutMany(blks []blocks.Block) (err error) {
	for _, bs := range m {
		if err = bs.PutMany(blks); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) DeleteBlock(cid cid.Cid) (err error) {
	for _, bs := range m {
		if err = bs.DeleteBlock(cid); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) DeleteMany(cids []cid.Cid) (err error) {
	for _, bs := range m {
		if err = bs.DeleteMany(cids); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	// this does not deduplicate; this interface needs to be revisited.
	outCh := make(chan cid.Cid)

	go func() {
		defer close(outCh)

		for _, bs := range m {
			ch, err := bs.AllKeysChan(ctx)
			if err != nil {
				return
			}
			for cid := range ch {
				outCh <- cid
			}
		}
	}()

	return outCh, nil
}

func (m unionBlockstore) HashOnRead(enabled bool) {
	for _, bs := range m {
		bs.HashOnRead(enabled)
	}
}
