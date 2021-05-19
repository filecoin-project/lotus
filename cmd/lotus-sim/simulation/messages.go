package simulation

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

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

func (nd *Node) storeMessages(ctx context.Context, messages []*types.Message) (cid.Cid, error) {
	var blsMessages, sekpMessages []cid.Cid
	fakeSig := make([]byte, 32)
	for _, msg := range messages {
		protocol := msg.From.Protocol()

		// It's just a very convenient way to fill up accounts.
		if msg.From == builtin.BurntFundsActorAddr {
			protocol = address.SECP256K1
		}
		switch protocol {
		case address.SECP256K1:
			chainMsg := &types.SignedMessage{
				Message: *msg,
				Signature: crypto.Signature{
					Type: crypto.SigTypeSecp256k1,
					Data: fakeSig,
				},
			}
			c, err := nd.Chainstore.PutMessage(chainMsg)
			if err != nil {
				return cid.Undef, err
			}
			sekpMessages = append(sekpMessages, c)
		case address.BLS:
			c, err := nd.Chainstore.PutMessage(msg)
			if err != nil {
				return cid.Undef, err
			}
			blsMessages = append(blsMessages, c)
		default:
			return cid.Undef, xerrors.Errorf("unexpected from address %q of type %d", msg.From, msg.From.Protocol())
		}
	}
	adtStore := nd.Chainstore.ActorStore(ctx)
	blsMsgArr, err := toArray(adtStore, blsMessages)
	if err != nil {
		return cid.Undef, err
	}
	sekpMsgArr, err := toArray(adtStore, sekpMessages)
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
