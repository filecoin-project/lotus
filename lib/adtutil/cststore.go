package adtutil

import (
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

func NewStore(ctx context.Context, cst *cbor.BasicIpldStore) adt.Store {
	return &store{
		cst: cst,
		ctx: ctx,
	}
}

type store struct {
	cst cbor.IpldStore
	ctx context.Context
}

func (a *store) Context() context.Context {
	return a.ctx
}

func (a *store) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	return a.cst.Get(ctx, c, out)
}

func (a *store) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return a.cst.Put(ctx, v)
}
