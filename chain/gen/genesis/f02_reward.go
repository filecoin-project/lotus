package genesis

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	"github.com/filecoin-project/go-state-types/manifest"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
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
		Balance: types.BigInt{Int: buildconstants.InitialRewardBalance},
		Head:    statecid,
	}

	return act, nil
}

// SetRewardSWA installs swa as the reward actor's FIP-0118 stream weight actor, holding its
// writes for timelock epochs.
func SetRewardSWA(ctx context.Context, st *state.StateTree, swa address.Address, timelock abi.ChainEpoch) error {
	act, err := st.GetActor(reward.Address)
	if err != nil {
		return xerrors.Errorf("getting reward actor: %w", err)
	}
	rst, err := reward.Load(adt.WrapStore(ctx, st.Store), act)
	if err != nil {
		return xerrors.Errorf("loading reward state: %w", err)
	}
	switch s := rst.GetState().(type) {
	case *reward19.State:
		s.SWAActor = swa
		s.SWATimelockEpochs = timelock
	default:
		return xerrors.Errorf("reward actor v%d predates the SWA", rst.ActorVersion())
	}
	act.Head, err = st.Store.Put(ctx, rst.GetState())
	if err != nil {
		return xerrors.Errorf("putting reward state: %w", err)
	}
	return st.SetActor(reward.Address, act)
}
