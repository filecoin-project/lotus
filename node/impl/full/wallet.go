package full

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"

	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type WalletAPI struct {
	fx.In

	StateManager *stmgr.StateManager
	Wallet       *wallet.Wallet
}

func (a *WalletAPI) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return a.Wallet.GenerateKey(typ)
}

func (a *WalletAPI) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return a.Wallet.HasKey(addr)
}

func (a *WalletAPI) WalletList(ctx context.Context) ([]address.Address, error) {
	return a.Wallet.ListAddrs()
}

func (a *WalletAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	return a.StateManager.GetBalance(addr, nil)
}

func (a *WalletAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*types.Signature, error) {
	return a.Wallet.Sign(ctx, k, msg)
}

func (a *WalletAPI) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	msgbytes, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	sig, err := a.WalletSign(ctx, k, msgbytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign message: %w", err)
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}

func (a *WalletAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	addrs, err := a.Wallet.ListAddrs()
	if err != nil {
		return address.Undef, err
	}
	if len(addrs) == 0 {
		return address.Undef, xerrors.New("no addresses in wallet")
	}

	// TODO: store a default address in the config or 'wallet' portion of the repo
	return addrs[0], nil
}

func (a *WalletAPI) WalletExport(ctx context.Context, addr address.Address) ([]byte, error) {
	return a.Wallet.Export(addr)
}

func (a *WalletAPI) WalletImport(ctx context.Context, b []byte) (address.Address, error) {
	return a.Wallet.Import(b)
}
