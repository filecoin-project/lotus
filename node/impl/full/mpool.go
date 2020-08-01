package full

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type MpoolAPI struct {
	fx.In

	WalletAPI
	GasAPI

	Chain *store.ChainStore

	Mpool *messagepool.MessagePool
}

func (a *MpoolAPI) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	pending, mpts := a.Mpool.Pending()

	haveCids := map[cid.Cid]struct{}{}
	for _, m := range pending {
		haveCids[m.Cid()] = struct{}{}
	}

	if ts == nil || mpts.Height() > ts.Height() {
		return pending, nil
	}

	for {
		if mpts.Height() == ts.Height() {
			if mpts.Equals(ts) {
				return pending, nil
			}
			// different blocks in tipsets

			have, err := a.Mpool.MessagesForBlocks(ts.Blocks())
			if err != nil {
				return nil, xerrors.Errorf("getting messages for base ts: %w", err)
			}

			for _, m := range have {
				haveCids[m.Cid()] = struct{}{}
			}
		}

		msgs, err := a.Mpool.MessagesForBlocks(ts.Blocks())
		if err != nil {
			return nil, xerrors.Errorf(": %w", err)
		}

		for _, m := range msgs {
			if _, ok := haveCids[m.Cid()]; ok {
				continue
			}

			haveCids[m.Cid()] = struct{}{}
			pending = append(pending, m)
		}

		if mpts.Height() >= ts.Height() {
			return pending, nil
		}

		ts, err = a.Chain.LoadTipSet(ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading parent tipset: %w", err)
		}
	}
}

func (a *MpoolAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return a.Mpool.Push(smsg)
}

// GasMargin sets by how much should gas used be increased over test execution
var GasMargin = 1.5

func (a *MpoolAPI) MpoolPushMessage(ctx context.Context, msg *types.Message) (*types.SignedMessage, error) {
	if msg.Nonce != 0 {
		return nil, xerrors.Errorf("MpoolPushMessage expects message nonce to be 0, was %d", msg.Nonce)
	}
	if msg.GasLimit == 0 {
		gasLimit, err := a.GasEstimateGasLimit(ctx, msg, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas used: %w", err)
		}
		msg.GasLimit = int64(float64(gasLimit) * GasMargin)
	}

	if msg.GasPrice == types.EmptyInt || types.BigCmp(msg.GasPrice, types.NewInt(0)) == 0 {
		gasPrice, err := a.GasEstimateGasPrice(ctx, 2, msg.From, msg.GasLimit, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("estimating gas price: %w", err)
		}
		msg.GasPrice = gasPrice
	}

	return a.Mpool.PushWithNonce(ctx, msg.From, func(from address.Address, nonce uint64) (*types.SignedMessage, error) {
		msg.Nonce = nonce
		if msg.From.Protocol() == address.ID {
			log.Warnf("Push from ID address (%s), adjusting to %s", msg.From, from)
			msg.From = from
		}

		b, err := a.WalletBalance(ctx, msg.From)
		if err != nil {
			return nil, xerrors.Errorf("mpool push: getting origin balance: %w", err)
		}

		if b.LessThan(msg.Value) {
			return nil, xerrors.Errorf("mpool push: not enough funds: %s < %s", b, msg.Value)
		}

		return a.WalletSignMessage(ctx, from, msg)
	})
}

func (a *MpoolAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}

func (a *MpoolAPI) MpoolSub(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	return a.Mpool.Updates(ctx)
}
