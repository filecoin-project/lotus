package util

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
)

// TODO extract this to a common location in lotus and reuse the code

// APIIpldStore is required for AMT and HAMT access.
type APIIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func NewAPIIpldStore(ctx context.Context, api api.FullNode) *APIIpldStore {
	return &APIIpldStore{
		ctx: ctx,
		api: api,
	}
}

func (ht *APIIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *APIIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("Object does not implement CBORUnmarshaler: %T", out)
}

func (ht *APIIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cid.Undef, fmt.Errorf("Put is not implemented on APIIpldStore")
}
