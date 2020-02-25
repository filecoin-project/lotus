package genesis

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/cron"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetupCronActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)
	cas := &cron.State{Entries: []cron.Entry{{
		Receiver:  builtin.StoragePowerActorAddr,
		MethodNum: builtin.MethodsPower.OnEpochTickEnd,
	},
	}}

	stcid, err := cst.Put(context.TODO(), cas)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    actors.CronCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}
