package genesis

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"

	"github.com/filecoin-project/specs-actors/v8/actors/builtin/manifest"
	system8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/system"

	cbor "github.com/ipfs/go-ipld-cbor"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetupSystemActor(ctx context.Context, bs bstore.Blockstore, av actors.Version) (*types.Actor, error) {

	cst := cbor.NewCborStore(bs)
	st, err := system.MakeState(adt.WrapStore(ctx, cst), av)
	if err != nil {
		return nil, err
	}

	if av >= actors.Version8 {
		mfCid, ok := actors.GetManifest(av)
		if !ok {
			return nil, xerrors.Errorf("missing manifest for actors version %d", av)
		}

		mf := manifest.Manifest{}
		if err := cst.Get(ctx, mfCid, &mf); err != nil {
			return nil, xerrors.Errorf("loading manifest for actors version %d: %w", av, err)
		}

		st8 := st.GetState().(*system8.State)
		st8.BuiltinActors = mf.Data
	}

	statecid, err := cst.Put(ctx, st.GetState())
	if err != nil {
		return nil, err
	}

	actcid, err := system.GetActorCodeID(av)
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
