package genesis

import (
	"context"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

func SetupStorageMarketActor(bs bstore.Blockstore, miners []genesis.Miner) (*types.Actor, error) {
	ctx := context.TODO()
	cst := cbor.NewCborStore(bs)
	ast := store.ActorStore(context.TODO(), bs)

	cdeals := make([]cbg.CBORMarshaler, len(deals))
	sdeals := make([]cbg.CBORMarshaler, len(deals))
	for i, deal := range deals {
		d := deal // copy

		cdeals[i] = &d
		sdeals[i] = &market.DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: -1,
			SlashEpoch:       -1,
		}
	}

	dealAmt, err := amt.FromArray(ctx, cst, cdeals)
	if err != nil {
		return nil, xerrors.Errorf("amt build failed: %w", err)
	}

	stateAmt, err := amt.FromArray(ctx, cst, sdeals)
	if err != nil {
		return nil, xerrors.Errorf("amt build failed: %w", err)
	}

	sms, err := market.ConstructState(ast)
	if err != nil {
		return nil, err
	}

	sms.Proposals = dealAmt
	sms.States = stateAmt

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	// TODO: MARKET BALANCES!!!!!!111

	act := &types.Actor{
		Code:    actors.StorageMarketCodeCid,
		Head:    stcid,
		Balance: types.NewInt(0),
	}

	return act, nil
}
