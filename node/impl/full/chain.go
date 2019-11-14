package full

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
)

type ChainAPI struct {
	fx.In

	WalletAPI

	Chain *store.ChainStore
}

func (a *ChainAPI) ChainNotify(ctx context.Context) (<-chan []*store.HeadChange, error) {
	return a.Chain.SubHeadChanges(ctx), nil
}

func (a *ChainAPI) ChainHead(context.Context) (*types.TipSet, error) {
	return a.Chain.GetHeaviestTipSet(), nil
}

func (a *ChainAPI) ChainGetRandomness(ctx context.Context, pts *types.TipSet, tickets []*types.Ticket, lb uint64) ([]byte, error) {
	return a.Chain.GetRandomness(ctx, pts.Cids(), tickets, lb)
}

func (a *ChainAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
}

func (a *ChainAPI) ChainGetTipSet(ctx context.Context, cids []cid.Cid) (*types.TipSet, error) {
	return a.Chain.LoadTipSet(cids)
}

func (a *ChainAPI) ChainGetBlockMessages(ctx context.Context, msg cid.Cid) (*api.BlockMessages, error) {
	b, err := a.Chain.GetBlock(msg)
	if err != nil {
		return nil, err
	}

	bmsgs, smsgs, err := a.Chain.MessagesForBlock(b)
	if err != nil {
		return nil, err
	}

	cids := make([]cid.Cid, len(bmsgs)+len(smsgs))

	for i, m := range bmsgs {
		cids[i] = m.Cid()
	}

	for i, m := range smsgs {
		cids[i+len(bmsgs)] = m.Cid()
	}

	return &api.BlockMessages{
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
		Cids:          cids,
	}, nil
}

func (a *ChainAPI) ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]api.Message, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
	}

	// genesis block has no parent messages...
	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := a.Chain.LoadTipSet(b.Parents)
	if err != nil {
		return nil, err
	}

	cm, err := a.Chain.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []api.Message
	for _, m := range cm {
		out = append(out, api.Message{
			Cid:     m.Cid(),
			Message: m.VMMessage(),
		})
	}

	return out, nil
}

func (a *ChainAPI) ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
	}

	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := a.Chain.LoadTipSet(b.Parents)
	if err != nil {
		return nil, err
	}

	cm, err := a.Chain.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []*types.MessageReceipt
	for i := 0; i < len(cm); i++ {
		r, err := a.Chain.GetParentReceipt(b, i)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, nil
}

func (a *ChainAPI) ChainGetTipSetByHeight(ctx context.Context, h uint64, ts *types.TipSet) (*types.TipSet, error) {
	return a.Chain.GetTipsetByHeight(ctx, h, ts)
}

func (a *ChainAPI) ChainReadObj(ctx context.Context, obj cid.Cid) ([]byte, error) {
	blk, err := a.Chain.Blockstore().Get(obj)
	if err != nil {
		return nil, xerrors.Errorf("blockstore get: %w", err)
	}

	return blk.RawData(), nil
}

func (a *ChainAPI) ChainSetHead(ctx context.Context, ts *types.TipSet) error {
	return a.Chain.SetHead(ts)
}

func (a *ChainAPI) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	genb, err := a.Chain.GetGenesis()
	if err != nil {
		return nil, err
	}

	return types.NewTipSet([]*types.BlockHeader{genb})
}

func (a *ChainAPI) ChainTipSetWeight(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	return a.Chain.Weight(ctx, ts)
}
