package blockstore

import (
	"context"
	"os"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

// buflog is a logger for the buffered blockstore. It is subscoped from the
// blockstore logger.
var buflog = log.Named("buf")

type BufferedBlockstore struct {
	read  Blockstore
	write Blockstore
}

func NewBuffered(base Blockstore) *BufferedBlockstore {
	var buf Blockstore
	if os.Getenv("LOTUS_DISABLE_VM_BUF") == "iknowitsabadidea" {
		buflog.Warn("VM BLOCKSTORE BUFFERING IS DISABLED")
		buf = base
	} else {
		buf = NewMemory()
	}

	bs := &BufferedBlockstore{
		read:  base,
		write: buf,
	}
	return bs
}

func NewTieredBstore(r Blockstore, w Blockstore) *BufferedBlockstore {
	return &BufferedBlockstore{
		read:  r,
		write: w,
	}
}

var (
	_ Blockstore = (*BufferedBlockstore)(nil)
	_ Viewer     = (*BufferedBlockstore)(nil)
)

func (bs *BufferedBlockstore) Flush(ctx context.Context) error { return bs.write.Flush(ctx) }

func (bs *BufferedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
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

func (bs *BufferedBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	if err := bs.read.DeleteBlock(ctx, c); err != nil {
		return err
	}

	return bs.write.DeleteBlock(ctx, c)
}

func (bs *BufferedBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	if err := bs.read.DeleteMany(ctx, cids); err != nil {
		return err
	}

	return bs.write.DeleteMany(ctx, cids)
}

func (bs *BufferedBlockstore) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	// both stores are viewable.
	if err := bs.write.View(ctx, c, callback); !ipld.IsNotFound(err) {
		return err // propagate errors, or nil, i.e. found.
	} // else not found in write blockstore; fall through.
	return bs.read.View(ctx, c, callback)
}

func (bs *BufferedBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	if out, err := bs.write.Get(ctx, c); err != nil {
		if !ipld.IsNotFound(err) {
			return nil, err
		}
	} else {
		return out, nil
	}

	return bs.read.Get(ctx, c)
}

func (bs *BufferedBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	s, err := bs.read.GetSize(ctx, c)
	if ipld.IsNotFound(err) || s == 0 {
		return bs.write.GetSize(ctx, c)
	}

	return s, err
}

func (bs *BufferedBlockstore) Put(ctx context.Context, blk block.Block) error {
	has, err := bs.read.Has(ctx, blk.Cid()) // TODO: consider dropping this check
	if err != nil {
		return err
	}

	if has {
		return nil
	}

	return bs.write.Put(ctx, blk)
}

func (bs *BufferedBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	has, err := bs.write.Has(ctx, c)
	if err != nil {
		return false, err
	}
	if has {
		return true, nil
	}

	return bs.read.Has(ctx, c)
}

func (bs *BufferedBlockstore) PutMany(ctx context.Context, blks []block.Block) error {
	return bs.write.PutMany(ctx, blks)
}

func (bs *BufferedBlockstore) Read() Blockstore {
	return bs.read
}
