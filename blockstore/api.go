package blockstore

import (
	"context"

	lru "github.com/hashicorp/golang-lru/v2"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type ChainIO interface {
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainPutObj(context.Context, blocks.Block) error
}

type apiBlockstore struct {
	api   ChainIO
	cache *lru.Cache[cid.Cid, []byte]
}

// This blockstore is adapted in the constructor.
var _ BasicBlockstore = (*apiBlockstore)(nil)

func NewAPIBlockstore(cio ChainIO) Blockstore {
	lc, err := lru.New[cid.Cid, []byte](1024) // we mostly come here for short-lived CLI applications so 1024 is a compromise size
	if err != nil {
		panic(err) // should never happen, only errors if size is <= 0
	}
	bs := &apiBlockstore{
		api:   cio,
		cache: lc,
	}
	return Adapt(bs) // return an adapted blockstore.
}

func (a *apiBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	return xerrors.New("not supported")
}

func (a *apiBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if a.cache.Contains(c) {
		return true, nil
	}
	return a.api.ChainHasObj(ctx, c)
}

func (a *apiBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if bb, ok := a.cache.Get(c); ok {
		return blocks.NewBlockWithCid(bb, c)
	}
	bb, err := a.api.ChainReadObj(ctx, c)
	if err != nil {
		return nil, err
	}
	a.cache.ContainsOrAdd(c, bb)
	return blocks.NewBlockWithCid(bb, c)
}

func (a *apiBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if bb, ok := a.cache.Peek(c); ok {
		return len(bb), nil
	}
	bb, err := a.api.ChainReadObj(ctx, c)
	if err != nil {
		return 0, err
	}
	a.cache.ContainsOrAdd(c, bb)
	return len(bb), nil
}

func (a *apiBlockstore) Put(ctx context.Context, block blocks.Block) error {
	a.cache.Add(block.Cid(), block.RawData())
	return a.api.ChainPutObj(ctx, block)
}

func (a *apiBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	for _, block := range blocks {
		a.cache.Add(block.Cid(), block.RawData())
		err := a.api.ChainPutObj(ctx, block)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *apiBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, xerrors.New("not supported")
}
