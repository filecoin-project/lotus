package api

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

type Wallet interface {
	WalletNew(context.Context, types.KeyType) (address.Address, error) //perm:admin
	WalletHas(context.Context, address.Address) (bool, error)          //perm:admin
	WalletList(context.Context) ([]address.Address, error)             //perm:admin

	WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta types.MsgSigningMeta) (*crypto.Signature, error) //perm:admin

	WalletExport(context.Context, address.Address) (*types.KeyInfo, error) //perm:admin
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error) //perm:admin
	WalletDelete(context.Context, address.Address) error                   //perm:admin
}
