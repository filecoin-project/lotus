package genesis

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStorageMarketActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)
	ast := store.ActorStore(context.TODO(), bs)

	sms, err := market.ConstructState(ast)
	if err != nil {
		return nil, err
	}

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code:    actors.StorageMarketCodeCid,
		Head:    stcid,
		Balance: types.NewInt(0),
	}

	return act, nil
}
