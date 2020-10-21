package full

import (
	"context"

	"go.opencensus.io/tag"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/metrics"
)

type WalletAPI struct {
	fx.In

	StateManagerAPI stmgr.StateManagerAPI
	Default         wallet.Default
	api.WalletAPI
}

func (a *WalletAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletBalance"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	act, err := a.StateManagerAPI.LoadActorTsk(ctx, addr, types.EmptyTSK)
	if xerrors.Is(err, types.ErrActorNotFound) {
		return big.Zero(), nil
	} else if err != nil {
		return big.Zero(), err
	}
	return act.Balance, nil
}

func (a *WalletAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*crypto.Signature, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletSign"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	keyAddr, err := a.StateManagerAPI.ResolveToKeyAddress(ctx, k, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve ID address: %w", keyAddr)
	}
	return a.WalletAPI.WalletSign(ctx, keyAddr, msg, api.MsgMeta{
		Type: api.MTUnknown,
	})
}

func (a *WalletAPI) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletSignMessage"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	keyAddr, err := a.StateManagerAPI.ResolveToKeyAddress(ctx, k, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve ID address: %w", keyAddr)
	}

	mb, err := msg.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing message: %w", err)
	}

	sig, err := a.WalletAPI.WalletSign(ctx, k, mb.Cid().Bytes(), api.MsgMeta{
		Type:  api.MTChainMsg,
		Extra: mb.RawData(),
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to sign message: %w", err)
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}

func (a *WalletAPI) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletVerify"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return sigs.Verify(sig, k, msg) == nil, nil
}

func (a *WalletAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletDefaultAddress"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Default.GetDefault()
}

func (a *WalletAPI) WalletSetDefault(ctx context.Context, addr address.Address) error {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletSetDefault"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Default.SetDefault(addr)
}

func (a *WalletAPI) WalletValidateAddress(ctx context.Context, str string) (address.Address, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "WalletValidateAddress"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return address.NewFromString(str)
}
