package genesis

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	bstore "github.com/brossetti1/lotus/blockstore"
	"github.com/brossetti1/lotus/chain/actors"
	"github.com/brossetti1/lotus/chain/actors/builtin/power"
	"github.com/brossetti1/lotus/chain/types"
)

func SetupStoragePowerActor(ctx context.Context, bs bstore.Blockstore, av actorstypes.Version) (*types.Actor, error) {

	cst := cbor.NewCborStore(bs)
	pst, err := power.MakeState(adt.WrapStore(ctx, cbor.NewCborStore(bs)), av)
	if err != nil {
		return nil, err
	}

	statecid, err := cst.Put(ctx, pst.GetState())
	if err != nil {
		return nil, err
	}

	actcid, ok := actors.GetActorCodeID(av, manifest.PowerKey)
	if !ok {
		return nil, xerrors.Errorf("failed to get power actor code ID for actors version %d", av)
	}

	act := &types.Actor{
		Code:    actcid,
		Head:    statecid,
		Balance: big.Zero(),
	}

	return act, nil
}
