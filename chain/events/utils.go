package events

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func (e *calledEvents) CheckMsg(ctx context.Context, smsg store.ChainMsg, hnd CalledHandler) CheckFunc {
	msg := smsg.VMMessage()

	return func(ts *types.TipSet) (done bool, more bool, err error) {
		fa, err := e.cs.StateGetActor(ctx, msg.From, ts.Key())
		if err != nil {
			return false, true, err
		}

		// >= because actor nonce is actually the next nonce that is expected to appear on chain
		if msg.Nonce >= fa.Nonce {
			return false, true, nil
		}

		rec, err := e.cs.StateGetReceipt(ctx, smsg.VMMessage().Cid(), ts.Key())
		if err != nil {
			return false, true, xerrors.Errorf("getting receipt in CheckMsg: %w", err)
		}

		more, err = hnd(msg, rec, ts, ts.Height())

		return true, more, err
	}
}

func (e *calledEvents) MatchMsg(inmsg *types.Message) MatchFunc {
	return func(msg *types.Message) (bool, error) {
		if msg.From == inmsg.From && msg.Nonce == inmsg.Nonce && !inmsg.Equals(msg) {
			return false, xerrors.Errorf("matching msg %s from %s, nonce %d: got duplicate origin/nonce msg %s", inmsg.Cid(), inmsg.From, inmsg.Nonce, msg.Nonce)
		}

		return inmsg.Equals(msg), nil
	}
}
