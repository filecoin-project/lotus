package lf3

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/lotus/api"
	"golang.org/x/xerrors"
)

type signer struct {
	wallet api.Wallet
}

// Signs a message with the secret key corresponding to a public key.
func (s *signer) Sign(sender gpbft.PubKey, msg []byte) ([]byte, error) {
	addr, err := address.NewBLSAddress(sender)
	if err != nil {
		return nil, xerrors.Errorf("converting pubkey to address: %w", err)
	}
	sig, err := s.wallet.WalletSign(context.TODO(), addr, msg, api.MsgMeta{Type: api.MTUnknown})
	if err != nil {
		return nil, xerrors.Errorf("error while signing: %w", err)
	}
	return sig.Data, nil
}

// MarshalPayloadForSigning marshals the given payload into the bytes that should be signed.
// This should usually call `Payload.MarshalForSigning(NetworkName)` except when testing as
// that method is slow (computes a merkle tree that's necessary for testing).
// Implementations must be safe for concurrent use.
func (s *signer) MarshalPayloadForSigning(nn gpbft.NetworkName, p *gpbft.Payload) []byte {
	return p.MarshalForSigning(nn)
}
