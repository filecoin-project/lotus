package genesis

import (
	"context"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func SetupRewardActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)

	st := reward.ConstructState()
	hcid, err := cst.Put(context.TODO(), st)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    builtin.RewardActorCodeID,
		Balance: types.BigInt{Int: build.InitialRewardBalance},
		Head:    hcid,
	}, nil
}
