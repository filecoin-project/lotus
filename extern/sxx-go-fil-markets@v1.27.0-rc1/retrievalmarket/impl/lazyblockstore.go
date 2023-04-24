package retrievalimpl

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/dagstore"
)

// lazyBlockstore is a read-only wrapper around a Blockstore that is loaded
// lazily when one of its methods are called
type lazyBlockstore struct {
	lk   sync.Mutex
	bs   dagstore.ReadBlockstore
	load func() (dagstore.ReadBlockstore, error)
}

func newLazyBlockstore(load func() (dagstore.ReadBlockstore, error)) *lazyBlockstore {
	return &lazyBlockstore{
		load: load,
	}
}

func (l *lazyBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	panic("cannot call DeleteBlock on read-only blockstore")
}

func (l *lazyBlockstore) Put(ctx context.Context, block blocks.Block) error {
	panic("cannot call Put on read-only blockstore")
}

func (l *lazyBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	panic("cannot call PutMany on read-only blockstore")
}

func (l *lazyBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	bs, err := l.init()
	if err != nil {
		return false, err
	}
	return bs.Has(ctx, c)
}

func (l *lazyBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	bs, err := l.init()
	if err != nil {
		return nil, err
	}
	return bs.Get(ctx, c)
}

func (l *lazyBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	bs, err := l.init()
	if err != nil {
		return 0, err
	}
	return bs.GetSize(ctx, c)
}

func (l *lazyBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	bs, err := l.init()
	if err != nil {
		return nil, err
	}
	return bs.AllKeysChan(ctx)
}

func (l *lazyBlockstore) HashOnRead(enabled bool) {
	bs, err := l.init()
	if err != nil {
		return
	}
	bs.HashOnRead(enabled)
}

func (l *lazyBlockstore) init() (dagstore.ReadBlockstore, error) {
	l.lk.Lock()
	defer l.lk.Unlock()

	if l.bs == nil {
		var err error
		l.bs, err = l.load()
		if err != nil {
			return nil, err
		}
	}
	return l.bs, nil
}

var _ bstore.Blockstore = (*lazyBlockstore)(nil)
