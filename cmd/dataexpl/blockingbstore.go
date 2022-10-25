package main

import (
	"context"
	"fmt"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

type blockReadBs struct {
	lk sync.Mutex

	finalized chan struct{}
	waiting   map[string]chan struct{} // mh -> chan

	sub bstore.Blockstore
}

func WithNoBlock(ctx context.Context) context.Context {
	return context.WithValue(ctx, "bbs-noblock", true)
}

func (b *blockReadBs) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	return b.sub.DeleteBlock(ctx, cid)
}

func (b *blockReadBs) waitForMh(ctx context.Context, mh multihash.Multihash) error {
	if ctx.Value("bbs-noblock") != nil && ctx.Value("bbs-noblock").(bool) {
		return nil
	}

	b.lk.Lock()
	var ok bool
	var wch chan struct{}

	if wch, ok = b.waiting[string(mh)]; !ok {
		wch = make(chan struct{})
		b.waiting[string(mh)] = wch
	}
	b.lk.Unlock()

	select {
	case <-wch:
	case <-b.finalized:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (b *blockReadBs) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	has, err := b.sub.Has(ctx, cid)
	if err != nil {
		return false, err
	}
	if !has {
		fmt.Println("wait has")
		if err := b.waitForMh(ctx, cid.Hash()); err != nil {
			return false, xerrors.Errorf("wait has: %w", err)
		}

		has, err = b.sub.Has(ctx, cid)
		if err != nil {
			return false, err
		}
	}

	return has, err
}

func (b *blockReadBs) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	fmt.Println("blocking get ", c)

	blk, err := b.sub.Get(ctx, c)
	if err == nil {
		return blk, err
	}
	if ipld.IsNotFound(err) {
		fmt.Println("wait get ", c)
		if err := b.waitForMh(ctx, c.Hash()); err != nil {
			return nil, err
		}

		fmt.Println("get avail ", c)

		return b.sub.Get(ctx, c)
	}
	return blk, err
}

func (b *blockReadBs) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	blk, err := b.sub.GetSize(ctx, c)
	if err == nil {
		return blk, err
	}
	if ipld.IsNotFound(err) {
		fmt.Println("wait get")
		if err := b.waitForMh(ctx, c.Hash()); err != nil {
			return 0, err
		}

		return b.sub.GetSize(ctx, c)
	}
	return blk, err
}

func (b *blockReadBs) availableMh(mh multihash.Multihash) {
	select {
	case <-b.finalized:
	case <-b.waiting[string(mh)]:

	default:
		if wch, ok := b.waiting[string(mh)]; ok {
			fmt.Println("avail md ", mh)
			close(wch)
		}
	}
}

func (b *blockReadBs) Put(ctx context.Context, block blocks.Block) error {
	fmt.Println("blocking put ", block.Cid())

	if err := b.sub.Put(ctx, block); err != nil {
		return err
	}

	b.lk.Lock()
	b.availableMh(block.Cid().Hash())
	b.lk.Unlock()

	return nil
}

func (b *blockReadBs) PutMany(ctx context.Context, blocks []blocks.Block) error {
	if err := b.sub.PutMany(ctx, blocks); err != nil {
		return err
	}

	b.lk.Lock()
	for _, block := range blocks {
		b.availableMh(block.Cid().Hash())
	}
	b.lk.Unlock()

	return nil
}

func (b *blockReadBs) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.sub.AllKeysChan(ctx)
}

func (b *blockReadBs) HashOnRead(enabled bool) {
	b.sub.HashOnRead(enabled)
}

func (b *blockReadBs) Finalize() {
	close(b.finalized)
}

func (b *blockReadBs) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	err := b.sub.View(ctx, c, callback)
	if err == nil {
		return err
	}
	if ipld.IsNotFound(err) {
		fmt.Println("wait view")
		if err := b.waitForMh(ctx, c.Hash()); err != nil {
			return err
		}

		return b.sub.View(ctx, c, callback)
	}
	return err
}

func (b *blockReadBs) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	return b.DeleteMany(ctx, cids)
}

var _ bstore.Blockstore = &blockReadBs{}
