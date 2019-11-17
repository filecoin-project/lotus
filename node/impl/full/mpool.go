package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
)

type MpoolAPI struct {
	fx.In

	WalletAPI

	Mpool *chain.MessagePool
}

func (a *MpoolAPI) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	// TODO: need to make sure we don't return messages that were already included in the referenced chain
	// also need to accept ts == nil just fine, assume nil == chain.Head()
	return a.Mpool.Pending(), nil
}

func (a *MpoolAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	return a.Mpool.Push(smsg)
}

func (a *MpoolAPI) MpoolPushMessage(ctx context.Context, msg *types.Message) (*types.SignedMessage, error) {
	if msg.Nonce != 0 {
		return nil, xerrors.Errorf("MpoolPushMessage expects message nonce to be 0, was %d", msg.Nonce)
	}

	return a.Mpool.PushWithNonce(msg.From, func(nonce uint64) (*types.SignedMessage, error) {
		msg.Nonce = nonce

		b, err := a.WalletBalance(ctx, msg.From)
		if err != nil {
			return nil, xerrors.Errorf("mpool push: getting origin balance: %w", err)
		}

		if b.LessThan(msg.Value) {
			return nil, xerrors.Errorf("mpool push: not enough funds: %s < %s", b, msg.Value)
		}

		return a.WalletSignMessage(ctx, msg.From, msg)
	})
}

func (a *MpoolAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}

func (a *MpoolAPI) MpoolSub(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	return a.Mpool.Updates(ctx)
}
