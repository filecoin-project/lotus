package full

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
)

type ChainAPI struct {
	fx.In

	WalletAPI

	Chain  *store.ChainStore
	PubSub *pubsub.PubSub
}

func (a *ChainAPI) ChainNotify(ctx context.Context) (<-chan []*store.HeadChange, error) {
	return a.Chain.SubHeadChanges(ctx), nil
}

func (a *ChainAPI) ChainSubmitBlock(ctx context.Context, blk *types.BlockMsg) error {
	if err := a.Chain.AddBlock(blk.Header); err != nil {
		return xerrors.Errorf("AddBlock failed: %w", err)
	}

	b, err := blk.Serialize()
	if err != nil {
		return xerrors.Errorf("serializing block for pubsub publishing failed: %w", err)
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}

func (a *ChainAPI) ChainHead(context.Context) (*types.TipSet, error) {
	return a.Chain.GetHeaviestTipSet(), nil
}

func (a *ChainAPI) ChainGetRandomness(ctx context.Context, pts *types.TipSet, tickets []*types.Ticket, lb int) ([]byte, error) {
	return a.Chain.GetRandomness(ctx, pts.Cids(), tickets, int64(lb))
}

func (a *ChainAPI) ChainWaitMsg(ctx context.Context, msg cid.Cid) (*api.MsgWait, error) {
	// TODO: consider using event system for this, expose confidence

	recpt, err := a.Chain.WaitForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &api.MsgWait{
		Receipt: *recpt,
	}, nil
}

func (a *ChainAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
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

func (a *ChainAPI) ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]cid.Cid, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
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

	var out []cid.Cid
	for _, m := range cm {
		out = append(out, m.Cid())
	}

	return out, nil
}

func (a *ChainAPI) ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
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
