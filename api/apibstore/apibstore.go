package apibstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"
)

type ChainIO interface {
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
}

type apiBStore struct {
	api ChainIO
}

func (a *apiBStore) DeleteBlock(cid.Cid) error {
	return xerrors.New("not supported")
}

func (a *apiBStore) Has(c cid.Cid) (bool, error) {
	return a.api.ChainHasObj(context.TODO(), c)
}

func (a *apiBStore) Get(c cid.Cid) (blocks.Block, error) {
	bb, err := a.api.ChainReadObj(context.TODO(), c)
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(bb, c)
}

func (a *apiBStore) GetSize(c cid.Cid) (int, error) {
	bb, err := a.api.ChainReadObj(context.TODO(), c)
	if err != nil {
		return 0, err
	}
	return len(bb), nil
}

func (a *apiBStore) Put(blocks.Block) error {
	return xerrors.New("not supported")
}

func (a *apiBStore) PutMany([]blocks.Block) error {
	return xerrors.New("not supported")
}

func (a *apiBStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, xerrors.New("not supported")
}

func (a *apiBStore) HashOnRead(enabled bool) {
	return
}

var _ blockstore.Blockstore = &apiBStore{}
