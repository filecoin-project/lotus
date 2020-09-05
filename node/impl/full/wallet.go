package full

import (
	"context"

	"github.com/filecoin-project/go-address"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/sigs"
)

type WalletAPI struct {
	fx.In

	StateManager *stmgr.StateManager
	Default      wallet.Default
	api.WalletAPI
}

func (a *WalletAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	var bal types.BigInt
	err := a.StateManager.WithParentStateTsk(types.EmptyTSK, a.StateManager.WithActor(addr, func(act *types.Actor) error {
		bal = act.Balance
		return nil
	}))

	if xerrors.Is(err, types.ErrActorNotFound) {
		return big.Zero(), nil
	}
	return bal, err
}

func (a *WalletAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*crypto.Signature, error) {
	keyAddr, err := a.StateManager.ResolveToKeyAddress(ctx, k, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve ID address: %w", keyAddr)
	}
	return a.WalletSign(ctx, keyAddr, msg)
}

func (a *WalletAPI) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	keyAddr, err := a.StateManager.ResolveToKeyAddress(ctx, k, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve ID address: %w", keyAddr)
	}
	return a.WalletAPI.WalletSignMessage(ctx, keyAddr, msg)
}

func (a *WalletAPI) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) bool {
	return sigs.Verify(sig, k, msg) == nil
}

func (a *WalletAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	return a.Default.GetDefault()
}

func (a *WalletAPI) WalletSetDefault(ctx context.Context, addr address.Address) error {
	return a.Default.SetDefault(addr)
}
