package api

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

type SignFunc = func(context.Context, []byte) (*types.Signature, error)

type Signer func(context.Context, address.Address, []byte) (*types.Signature, error)

type Signable interface {
	Sign(context.Context, SignFunc) error
}

func SignWith(ctx context.Context, signer Signer, addr address.Address, signable ...Signable) error {
	for _, s := range signable {
		err := s.Sign(ctx, func(ctx context.Context, b []byte) (*types.Signature, error) {
			return signer(ctx, addr, b)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
