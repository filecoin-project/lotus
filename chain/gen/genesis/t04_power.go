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
		TotalRawBytePower:       big.NewInt(0),
		TotalBytesCommitted:     big.NewInt(0),
		TotalQualityAdjPower:    big.NewInt(0),
		TotalQABytesCommitted:   big.NewInt(0),
		TotalPledgeCollateral:   big.NewInt(0),
		MinerCount:              0,
		MinerAboveMinPowerCount: 0,
		CronEventQueue:          emptyhamt,
		LastEpochTick:           0,
		Claims:                  emptyhamt,
		ProofValidationBatch:    nil,
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
