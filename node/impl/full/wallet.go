package full

import (
	"context"

	"github.com/filecoin-project/go-address"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/sigs"
)

type WalletAPI struct {
	fx.In

	StateManagerAPI stmgr.StateManagerAPI
	Wallet          *wallet.Wallet
}

func (a *WalletAPI) WalletNew(ctx context.Context, typ crypto.SigType) (address.Address, error) {
	return a.Wallet.GenerateKey(typ)
}

func (a *WalletAPI) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return a.Wallet.HasKey(addr)
}

func (a *WalletAPI) WalletList(ctx context.Context) ([]address.Address, error) {
	return a.Wallet.ListAddrs()
}

func (a *WalletAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	act, err := a.StateManagerAPI.LoadActorTsk(ctx, addr, types.EmptyTSK)
	if xerrors.Is(err, types.ErrActorNotFound) {
		return big.Zero(), nil
	} else if err != nil {
		return big.Zero(), err
	}
	return act.Balance, nil
}

func (a *WalletAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*crypto.Signature, error) {
	keyAddr, err := a.StateManagerAPI.ResolveToKeyAddress(ctx, k, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve ID address: %w", keyAddr)
	}
	return a.Wallet.Sign(ctx, keyAddr, msg)
}

func (a *WalletAPI) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	mcid := msg.Cid()

	sig, err := a.WalletSign(ctx, k, mcid.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to sign message: %w", err)
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}

func (a *WalletAPI) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	return sigs.Verify(sig, k, msg) == nil, nil
}

func (a *WalletAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	return a.Wallet.GetDefault()
}

func (a *WalletAPI) WalletSetDefault(ctx context.Context, addr address.Address) error {
	return a.Wallet.SetDefault(addr)
}

func (a *WalletAPI) WalletExport(ctx context.Context, addr address.Address) (*types.KeyInfo, error) {
	return a.Wallet.Export(addr)
}

func (a *WalletAPI) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return a.Wallet.Import(ki)
}

func (a *WalletAPI) WalletDelete(ctx context.Context, addr address.Address) error {
	return a.Wallet.DeleteKey(addr)
}

func (a *WalletAPI) WalletValidateAddress(ctx context.Context, str string) (address.Address, error) {
	return address.NewFromString(str)
}
