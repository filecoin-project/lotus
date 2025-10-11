package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

type unionBlockstore []Blockstore

// Union returns an unioned blockstore.
//
//   - Reads return from the first blockstore that has the value, querying in the
//     supplied order.
//   - Writes (puts and deletes) are broadcast to all stores.
func Union(stores ...Blockstore) Blockstore {
	return unionBlockstore(stores)
}

func (m unionBlockstore) Has(ctx context.Context, cid cid.Cid) (has bool, err error) {
	for _, bs := range m {
		if has, err = bs.Has(ctx, cid); has || err != nil {
			break
		}
	}
	return has, err
}

func (m unionBlockstore) Get(ctx context.Context, cid cid.Cid) (blk blocks.Block, err error) {
	for _, bs := range m {
		if blk, err = bs.Get(ctx, cid); err == nil || !ipld.IsNotFound(err) {
			break
		}
	}
	return blk, err
}

func (m unionBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) (err error) {
	for _, bs := range m {
		if err = bs.View(ctx, cid, callback); err == nil || !ipld.IsNotFound(err) {
			break
		}
	}
	return err
}

func (m unionBlockstore) GetSize(ctx context.Context, cid cid.Cid) (size int, err error) {
	for _, bs := range m {
		if size, err = bs.GetSize(ctx, cid); err == nil || !ipld.IsNotFound(err) {
			break
		}
	}
	return size, err
}

func (m unionBlockstore) Flush(ctx context.Context) (err error) {
	for _, bs := range m {
		if err = bs.Flush(ctx); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) Put(ctx context.Context, block blocks.Block) (err error) {
	for _, bs := range m {
		if err = bs.Put(ctx, block); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) PutMany(ctx context.Context, blks []blocks.Block) (err error) {
	for _, bs := range m {
		if err = bs.PutMany(ctx, blks); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) DeleteBlock(ctx context.Context, cid cid.Cid) (err error) {
	for _, bs := range m {
		if err = bs.DeleteBlock(ctx, cid); err != nil {
			break
		}
	}
	return err
}

func (m unionBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) (err error) {
	for _, bs := range m {
		if err = bs.DeleteMany(ctx, cids); err != nil {
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
