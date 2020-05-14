package genesis

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/filecoin-project/lotus/chain/types"
)

var RootVerifierAddr address.Address

var RootVerifierID address.Address

func init() {
	k, err := address.NewFromString("t3qfoulel6fy6gn3hjmbhpdpf6fs5aqjb5fkurhtwvgssizq4jey5nw4ptq5up6h7jk7frdvvobv52qzmgjinq")
	if err != nil {
		panic(err)
	}

	RootVerifierAddr = k

	idk, err := address.NewFromString("t080")
	if err != nil {
		panic(err)
	}

	RootVerifierID = idk
}

func SetupVerifiedRegistryActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := cbor.NewCborStore(bs)

	h, err := cst.Put(context.TODO(), hamt.NewNode(cst, hamt.UseTreeBitWidth(5)))
	if err != nil {
		return nil, err
	}

	sms := verifreg.ConstructState(h, RootVerifierID)

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
