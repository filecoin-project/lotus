package v0api

import (
	"github.com/filecoin-project/lotus/api/v1api"
)

type WrapperV1 struct {
	v1api.FullNode
}

/* example:
- dropped StateGetReceipt
- tsk param for StateSearchMsg

func (w *WrapperV1) StateSearchMsg(ctx context.Context, c cid.Cid) (*api.MsgLookup, error) {
	return w.FullNode.StateSearchMsg(ctx, c, types.EmptyTSK)
}

func (w *WrapperV1) StateGetReceipt(ctx context.Context, cid cid.Cid, key types.TipSetKey) (*types.MessageReceipt, error) {
	m, err := w.FullNode.StateSearchMsg(ctx, cid, key)
	if err != nil {
		return nil, err
	}

	if m == nil {
		return nil, nil
	}

	return &m.Receipt, nil
}*/

var _ FullNode = &WrapperV1{}
