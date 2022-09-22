package genesis

import (
	"context"

	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"

	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

var GovernorId address.Address

func init() {

	idk, err := address.NewFromString("t085")
	if err != nil {
		panic(err)
	}

	GovernorId = idk
}

func SetupDatacapActor(ctx context.Context, bs bstore.Blockstore, av actorstypes.Version) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)
	vst, err := datacap.MakeState(adt.WrapStore(ctx, cbor.NewCborStore(bs)), av, GovernorId, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}

	statecid, err := cst.Put(ctx, vst.GetState())
	if err != nil {
		return nil, err
	}

	actcid, ok := actors.GetActorCodeID(av, actors.DatacapKey)
	if !ok {
		return nil, xerrors.Errorf("failed to get verifreg actor code ID for actors version %d", av)
	}

	act := &types.Actor{
		Code:    actcid,
		Head:    statecid,
		Balance: big.Zero(),
	}

	return act, nil
}
