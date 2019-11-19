package events

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"
)

func (e *calledEvents) CheckMsg(ctx context.Context, msg *types.Message, hnd CalledHandler) CheckFunc {
	return func(ts *types.TipSet) (done bool, more bool, err error) {
		fa, err := e.cs.StateGetActor(ctx, msg.From, ts)
		if err != nil {
			return false, true, err
		}

		// TODO: probably want to look at the chain to make sure it's
		//  the right message, but this is probably good enough for now
		done = fa.Nonce >= msg.Nonce

		rec, err := e.cs.StateGetReceipt(ctx, msg.Cid(), ts)
		if err != nil {
			return false, true, err
		}

		more, err = hnd(msg, rec, ts, ts.Height())

		return done, more, err
	}
}

func(e *calledEvents) MatchMsg(inmsg *types.Message) MatchFunc {
	return func(msg *types.Message) (bool, error) {
		return inmsg.Equals(msg), nil
	}
}
