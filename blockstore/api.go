package blockstore

import (
	"context"

	"github.com/ipfs/go-cid"
	blocks "github.com/ipfs/go-libipfs/blocks"
	"golang.org/x/xerrors"
)

type ChainIO interface {
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainPutObj(context.Context, blocks.Block) error
}

type apiBlockstore struct {
	api ChainIO
}

// This blockstore is adapted in the constructor.
var _ BasicBlockstore = (*apiBlockstore)(nil)

func NewAPIBlockstore(cio ChainIO) Blockstore {
	bs := &apiBlockstore{api: cio}
	return Adapt(bs) // return an adapted blockstore.
}

func (a *apiBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	return xerrors.New("not supported")
}

func (a *apiBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	return a.api.ChainHasObj(ctx, c)
}

func (a *apiBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	bb, err := a.api.ChainReadObj(ctx, c)
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(bb, c)
}

func (a *apiBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	bb, err := a.api.ChainReadObj(ctx, c)
	if err != nil {
		return 0, err
	}
	return len(bb), nil
}

func (a *apiBlockstore) Put(ctx context.Context, block blocks.Block) error {
	return a.api.ChainPutObj(ctx, block)
}

func (a *apiBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	for _, block := range blocks {
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

func (a *apiBlockstore) HashOnRead(enabled bool) {
	return
}
