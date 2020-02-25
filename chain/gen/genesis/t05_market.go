package genesis

import (
	"context"
	"github.com/ipfs/go-hamt-ipld"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStorageMarketActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)

	a, err := amt.NewAMT(cst).Flush(context.TODO())
	if err != nil {
		return nil, err
	}
	h, err := cst.Put(context.TODO(), hamt.NewNode(cst, hamt.UseTreeBitWidth(5)))
	if err != nil {
		return nil, err
	}

	sms := market.ConstructState(a, h, h)

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code:    builtin.StorageMarketActorCodeID,
		Head:    stcid,
		Balance: types.NewInt(0),
	}

	return act, nil
}
