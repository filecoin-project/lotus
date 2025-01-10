package lf3

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/api"
)

var _ gpbft.Signer = (*signer)(nil)

type signer struct {
	wallet api.Wallet
}

// Sign signs a message with the private key corresponding to a public key.
// The key must be known by the wallet and be of BLS type.
func (s *signer) Sign(ctx context.Context, sender gpbft.PubKey, msg []byte) ([]byte, error) {
	addr, err := address.NewBLSAddress(sender)
	if err != nil {
		return nil, xerrors.Errorf("converting pubkey to address: %w", err)
	}
	sig, err := s.wallet.WalletSign(ctx, addr, msg, api.MsgMeta{Type: api.MTUnknown})
	if err != nil {
		return nil, xerrors.Errorf("error while signing: %w", err)
	}
	return sig.Data, nil
}
