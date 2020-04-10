package genesis

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStoragePowerActor(bs bstore.Blockstore) (*types.Actor, error) {
	ctx := context.TODO()
	cst := cbor.NewCborStore(bs)
	nd := hamt.NewNode(cst, hamt.UseTreeBitWidth(5))
	emptyhamt, err := cst.Put(ctx, nd)
	if err != nil {
		return nil, err
	}

	sms := &power.State{
		TotalQualityAdjPower:     big.NewInt(1), // TODO: has to be 1 initially to avoid div by zero. Kinda annoying, should find a way to fix
		MinerCount:               0,
		EscrowTable:              emptyhamt,
		CronEventQueue:           emptyhamt,
		PoStDetectedFaultMiners:  emptyhamt,
		Claims:                   emptyhamt,
		NumMinersMeetingMinPower: 0,
	}

	stcid, err := cst.Put(ctx, sms)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    builtin.StoragePowerActorCodeID,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}
