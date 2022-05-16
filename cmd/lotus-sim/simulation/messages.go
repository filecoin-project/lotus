package simulation

import (
	"context"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

// toArray converts the given set of CIDs to an AMT. This is usually used to pack messages into blocks.
func toArray(store blockadt.Store, cids []cid.Cid) (cid.Cid, error) {
	arr := blockadt.MakeEmptyArray(store)
	for i, c := range cids {
		oc := cbg.CborCid(c)
		if err := arr.Set(uint64(i), &oc); err != nil {
			return cid.Undef, err
		}
	}
	return arr.Root()
}

// storeMessages packs a set of messages into a types.MsgMeta and returns the resulting CID. The
// resulting CID is valid for the BlocKHeader's Messages field.
func (sim *Simulation) storeMessages(ctx context.Context, messages []*types.Message) (cid.Cid, error) {
	// We store all messages as "bls" messages so they're executed in-order. This ensures
	// accurate gas accounting. It also ensures we don't, e.g., try to fund a miner after we
	// fail a pre-commit...
	var msgCids []cid.Cid
	for _, msg := range messages {
		c, err := sim.Node.Chainstore.PutMessage(ctx, msg)
		if err != nil {
			return cid.Undef, err
		}
		msgCids = append(msgCids, c)
	}
	adtStore := sim.Node.Chainstore.ActorStore(ctx)
	blsMsgArr, err := toArray(adtStore, msgCids)
	if err != nil {
		return cid.Undef, err
	}
	sekpMsgArr, err := toArray(adtStore, nil)
	if err != nil {
		return cid.Undef, err
	}

	msgsCid, err := adtStore.Put(adtStore.Context(), &types.MsgMeta{
		BlsMessages:   blsMsgArr,
		SecpkMessages: sekpMsgArr,
	})
	if err != nil {
		return cid.Undef, err
	}
	return msgsCid, nil
}
