package genesis

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStoragePowerActor(bs bstore.Blockstore) (*types.Actor, error) {
	store := adt.WrapStore(context.TODO(), cbor.NewCborStore(bs))
	emptyhamt, err := adt.MakeEmptyMap(store).Root()
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
		FirstCronEpoch:          0,
		Claims:                  emptyhamt,
		ProofValidationBatch:    nil,
	}

	stcid, err := store.Put(store.Context(), sms)
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
