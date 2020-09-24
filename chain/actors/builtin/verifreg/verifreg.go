package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/cbor"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin1 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {
	builtin.RegisterActorState(builtin0.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})
	builtin.RegisterActorState(builtin1.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load1(store, root)
	})
}

var Address = builtin0.VerifiedRegistryActorAddr

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {
	case builtin0.VerifiedRegistryActorCodeID:
		return load0(store, act.Head)
	case builtin1.VerifiedRegistryActorCodeID:
		return load1(store, act.Head)
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler

	VerifiedClientDataCap(address.Address) (bool, abi.StoragePower, error)
	VerifierDataCap(address.Address) (bool, abi.StoragePower, error)
	ForEachVerifier(func(addr address.Address, dcap abi.StoragePower) error) error
	ForEachClient(func(addr address.Address, dcap abi.StoragePower) error) error
}
