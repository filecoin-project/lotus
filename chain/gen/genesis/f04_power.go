package genesis

import (
	"context"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	cbor "github.com/ipfs/go-ipld-cbor"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStoragePowerActor(ctx context.Context, bs bstore.Blockstore, av actors.Version) (*types.Actor, error) {

	cst := cbor.NewCborStore(bs)
	pst, err := power.MakeState(adt.WrapStore(ctx, cbor.NewCborStore(bs)), av)
	if err != nil {
		return nil, err
	}

	statecid, err := cst.Put(ctx, pst.GetState())
	if err != nil {
		return nil, err
	}

	actcid, err := power.GetActorCodeID(av)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code:    actcid,
		Head:    statecid,
		Balance: big.Zero(),
	}

	return act, nil
}
