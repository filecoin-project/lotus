package bufbstore

import (
	"context"
	"os"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("bufbs")

type BufferedBS struct {
	read  bstore.Blockstore
	write bstore.Blockstore
}

func NewBufferedBstore(base bstore.Blockstore) *BufferedBS {
	buf := bstore.NewBlockstore(ds.NewMapDatastore())
	if os.Getenv("LOTUS_DISABLE_VM_BUF") == "iknowitsabadidea" {
		log.Warn("VM BLOCKSTORE BUFFERING IS DISABLED")
		buf = base
	}

	return &BufferedBS{
		read:  base,
		write: buf,
	}
}

func NewTieredBstore(r bstore.Blockstore, w bstore.Blockstore) *BufferedBS {
	return &BufferedBS{
		read:  r,
		write: w,
	}
}

var _ (bstore.Blockstore) = &BufferedBS{}

func (bs *BufferedBS) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	a, err := bs.read.AllKeysChan(ctx)
	if err != nil {
		return nil, err
	}

	b, err := bs.write.AllKeysChan(ctx)
	if err != nil {
		return nil, err
	}

	out := make(chan cid.Cid)
	go func() {
		defer close(out)
		for a != nil || b != nil {
			select {
			case val, ok := <-a:
				if !ok {
					a = nil
				} else {
					select {
					case out <- val:
					case <-ctx.Done():
						return
					}
				}
			case val, ok := <-b:
				if !ok {
					b = nil
				} else {
					select {
					case out <- val:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out, nil
}

func (bs *BufferedBS) DeleteBlock(c cid.Cid) error {
	if err := bs.read.DeleteBlock(c); err != nil {
		return err
	}

	return bs.write.DeleteBlock(c)
}

func (bs *BufferedBS) Get(c cid.Cid) (block.Block, error) {
	if out, err := bs.read.Get(c); err != nil {
		if err != bstore.ErrNotFound {
			return nil, err
		}
	} else {
		return out, nil
	}

	return bs.write.Get(c)
}

func (bs *BufferedBS) GetSize(c cid.Cid) (int, error) {
	s, err := bs.read.GetSize(c)
	if err == bstore.ErrNotFound || s == 0 {
		return bs.write.GetSize(c)
	}

	return s, err
}

func (bs *BufferedBS) Put(blk block.Block) error {
	has, err := bs.read.Has(blk.Cid())
	if err != nil {
		return err
	}

	if has {
		return nil
	}

	return bs.write.Put(blk)
}

func (bs *BufferedBS) Has(c cid.Cid) (bool, error) {
	has, err := bs.read.Has(c)
	if err != nil {
		return false, err
	}
	if has {
		return true, nil
	}

	return bs.write.Has(c)
}

func (bs *BufferedBS) HashOnRead(hor bool) {
	bs.read.HashOnRead(hor)
	bs.write.HashOnRead(hor)
}

func (bs *BufferedBS) PutMany(blks []block.Block) error {
	return bs.write.PutMany(blks)
}

func (bs *BufferedBS) Read() bstore.Blockstore {
	return bs.read
}
