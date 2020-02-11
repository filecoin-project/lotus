package genesis

import (
	"context"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetupStorageMarketActor(bs bstore.Blockstore, sroot cid.Cid, deals []market.DealProposal) (cid.Cid, error) {
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
		return cid.Undef, xerrors.Errorf("amt build failed: %w", err)
	}

	stateAmt, err := amt.FromArray(ctx, cst, sdeals)
	if err != nil {
		return cid.Undef, xerrors.Errorf("amt build failed: %w", err)
	}

	sms, err := market.ConstructState(ast)
	if err != nil {
		return cid.Cid{}, err
	}

	sms.Proposals = dealAmt
	sms.States = stateAmt

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return cid.Undef, err
	}

	// TODO: MARKET BALANCES!!!!!!111

	act := &types.Actor{
		Code:    actors.StorageMarketCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}

	state, err := state.LoadStateTree(cst, sroot)
	if err != nil {
		return cid.Undef, xerrors.Errorf("making new state tree: %w", err)
	}

	if err := state.SetActor(actors.StorageMarketAddress, act); err != nil {
		return cid.Undef, xerrors.Errorf("set storage market actor: %w", err)
	}

	return state.Flush(context.TODO())
}
