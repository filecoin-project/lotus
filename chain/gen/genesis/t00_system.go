package genesis

import (
	"context"
	"github.com/filecoin-project/specs-actors/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func SetupSystemActor(bs bstore.Blockstore) (*types.Actor, error) {
	var st system.State

	cst := cbor.NewCborStore(bs)

	statecid, err := cst.Put(context.TODO(), &st)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code: builtin.SystemActorCodeID,
		Head: statecid,
	}

	return act, nil
}
