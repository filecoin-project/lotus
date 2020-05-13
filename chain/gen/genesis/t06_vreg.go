package genesis

import (
	"context"
	"crypto/rand"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/filecoin-project/lotus/chain/types"
)

func SetupVerifiedRegistryActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)

	h, err := cst.Put(context.TODO(), hamt.NewNode(cst, hamt.UseTreeBitWidth(5)))
	if err != nil {
		return nil, err
	}

	var r [32]byte // TODO: grab from genesis template
	_, _ = rand.Read(r[:])
	k, _ := address.NewSecp256k1Address(r[:])

	sms := verifreg.ConstructState(h, k)

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code:    builtin.VerifiedRegistryActorCodeID,
		Head:    stcid,
		Balance: types.NewInt(0),
	}

	return act, nil
}
