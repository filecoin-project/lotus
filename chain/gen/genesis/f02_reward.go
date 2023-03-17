package genesis

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"

	bstore "github.com/brossetti1/lotus/blockstore"
	"github.com/brossetti1/lotus/build"
	"github.com/brossetti1/lotus/chain/actors"
	"github.com/brossetti1/lotus/chain/actors/adt"
	"github.com/brossetti1/lotus/chain/actors/builtin/reward"
	"github.com/brossetti1/lotus/chain/types"
)

func SetupRewardActor(ctx context.Context, bs bstore.Blockstore, qaPower big.Int, av actorstypes.Version) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)
	rst, err := reward.MakeState(adt.WrapStore(ctx, cst), av, qaPower)
	if err != nil {
		return nil, err
	}

	statecid, err := cst.Put(ctx, rst.GetState())
	if err != nil {
		return nil, err
	}

	actcid, ok := actors.GetActorCodeID(av, manifest.RewardKey)
	if !ok {
		return nil, xerrors.Errorf("failed to get reward actor code ID for actors version %d", av)
	}

	act := &types.Actor{
		Code:    actcid,
		Balance: types.BigInt{Int: build.InitialRewardBalance},
		Head:    statecid,
	}

	return act, nil
}
