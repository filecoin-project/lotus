package full

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type MpoolAPI struct {
	fx.In

	WalletAPI
	GasAPI

	Chain *store.ChainStore

	Mpool *messagepool.MessagePool

	PushLocks *dtypes.MpoolLocker
}

func (a *MpoolAPI) MpoolGetConfig(context.Context) (*types.MpoolConfig, error) {
	return a.Mpool.GetConfig(), nil
}

func (a *MpoolAPI) MpoolSetConfig(ctx context.Context, cfg *types.MpoolConfig) error {
	return a.Mpool.SetConfig(cfg)
}

func (a *MpoolAPI) MpoolSelect(ctx context.Context, tsk types.TipSetKey, ticketQuality float64) ([]*types.SignedMessage, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return a.Mpool.SelectMessages(ts, ticketQuality)
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

func (a *MpoolAPI) MpoolClear(ctx context.Context, local bool) error {
	a.Mpool.Clear(local)
	return nil
}

func (a *MpoolAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return a.Mpool.Push(smsg)
}

func capGasFee(msg *types.Message, maxFee abi.TokenAmount) {
	if maxFee.Equals(big.Zero()) {
		return
	}

	gl := types.NewInt(uint64(msg.GasLimit))
	totalFee := types.BigMul(msg.GasFeeCap, gl)
	minerFee := types.BigMul(msg.GasPremium, gl)

	if totalFee.LessThanEqual(maxFee) {
		return
	}

	// scale chain/miner fee down proportionally to fit in our budget
	// TODO: there are probably smarter things we can do here to optimize
	//  message inclusion latency

	msg.GasFeeCap = big.Div(maxFee, gl)
	msg.GasPremium = big.Div(big.Div(big.Mul(minerFee, maxFee), totalFee), gl)
}

func (a *MpoolAPI) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	{
		fromA, err := a.Stmgr.ResolveToKeyAddress(ctx, msg.From, nil)
		if err != nil {
			return nil, xerrors.Errorf("getting key address: %w", err)
		}
		done, err := a.PushLocks.TakeLock(ctx, fromA)
		if err != nil {
			return nil, xerrors.Errorf("taking lock: %w", err)
		}
		defer done()
	}

	if msg.Nonce != 0 {
		return nil, xerrors.Errorf("MpoolPushMessage expects message nonce to be 0, was %d", msg.Nonce)
	}

	msg, err := a.GasAPI.GasEstimateMessageGas(ctx, msg, spec, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("GasEstimateMessageGas error: %w", err)
	}

	sign := func(from address.Address, nonce uint64) (*types.SignedMessage, error) {
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
	}

	var m *types.SignedMessage
again:
	m, err = a.Mpool.PushWithNonce(ctx, msg.From, sign)
	if err == messagepool.ErrTryAgain {
		log.Debugf("temporary failure while pushing message: %s; retrying", err)
		goto again
	}
	return m, err
}

func (a *MpoolAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}

func (a *MpoolAPI) MpoolSub(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	return a.Mpool.Updates(ctx)
}
