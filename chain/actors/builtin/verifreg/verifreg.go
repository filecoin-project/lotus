package verifreg

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-state-types/cbor"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	verifreg8 "github.com/filecoin-project/specs-actors/v7/actors/builtin/verifreg"
)

func init() {

	builtin.RegisterActorState(builtin0.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version0, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load0(store, root)
		})
	}

	builtin.RegisterActorState(builtin2.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load2(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version2, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load2(store, root)
		})
	}

	builtin.RegisterActorState(builtin3.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load3(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version3, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load3(store, root)
		})
	}

	builtin.RegisterActorState(builtin4.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load4(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version4, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load4(store, root)
		})
	}

	builtin.RegisterActorState(builtin5.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load5(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version5, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load5(store, root)
		})
	}

	builtin.RegisterActorState(builtin6.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load6(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version6, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load6(store, root)
		})
	}

	builtin.RegisterActorState(builtin7.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load7(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version7, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load7(store, root)
		})
	}

	builtin.RegisterActorState(builtin8.VerifiedRegistryActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load8(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version8, "verifiedregistry"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load8(store, root)
		})
	}

}

var (
	Address = builtin8.VerifiedRegistryActorAddr
	Methods = builtin8.MethodsVerifiedRegistry
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != "verifiedregistry" {
			return nil, xerrors.Errorf("actor code is not verifiedregistry: %s", name)
		}

		switch av {

		case actors.Version0:
			return load0(store, act.Head)

		case actors.Version2:
			return load2(store, act.Head)

		case actors.Version3:
			return load3(store, act.Head)

		case actors.Version4:
			return load4(store, act.Head)

		case actors.Version5:
			return load5(store, act.Head)

		case actors.Version6:
			return load6(store, act.Head)

		case actors.Version7:
			return load7(store, act.Head)

		case actors.Version8:
			return load8(store, act.Head)

		default:
			return nil, xerrors.Errorf("unknown actor version: %d", av)
		}
	}

	switch act.Code {

	case builtin0.VerifiedRegistryActorCodeID:
		return load0(store, act.Head)

	case builtin2.VerifiedRegistryActorCodeID:
		return load2(store, act.Head)

	case builtin3.VerifiedRegistryActorCodeID:
		return load3(store, act.Head)

	case builtin4.VerifiedRegistryActorCodeID:
		return load4(store, act.Head)

	case builtin5.VerifiedRegistryActorCodeID:
		return load5(store, act.Head)

	case builtin6.VerifiedRegistryActorCodeID:
		return load6(store, act.Head)

	case builtin7.VerifiedRegistryActorCodeID:
		return load7(store, act.Head)

	case builtin8.VerifiedRegistryActorCodeID:
		return load8(store, act.Head)

	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version, rootKeyAddress address.Address) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store, rootKeyAddress)

	case actors.Version2:
		return make2(store, rootKeyAddress)

	case actors.Version3:
		return make3(store, rootKeyAddress)

	case actors.Version4:
		return make4(store, rootKeyAddress)

	case actors.Version5:
		return make5(store, rootKeyAddress)

	case actors.Version6:
		return make6(store, rootKeyAddress)

	case actors.Version7:
		return make7(store, rootKeyAddress)

	case actors.Version8:
		return make8(store, rootKeyAddress)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, "verifiedregistry"); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.VerifiedRegistryActorCodeID, nil

	case actors.Version2:
		return builtin2.VerifiedRegistryActorCodeID, nil

	case actors.Version3:
		return builtin3.VerifiedRegistryActorCodeID, nil

	case actors.Version4:
		return builtin4.VerifiedRegistryActorCodeID, nil

	case actors.Version5:
		return builtin5.VerifiedRegistryActorCodeID, nil

	case actors.Version6:
		return builtin6.VerifiedRegistryActorCodeID, nil

	case actors.Version7:
		return builtin7.VerifiedRegistryActorCodeID, nil

	case actors.Version8:
		return builtin8.VerifiedRegistryActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

type RemoveDataCapProposal = verifreg8.RemoveDataCapProposal
type RemoveDataCapRequest = verifreg8.RemoveDataCapRequest
type RemoveDataCapParams = verifreg8.RemoveDataCapParams
type RmDcProposalID = verifreg8.RmDcProposalID

const SignatureDomainSeparation_RemoveDataCap = verifreg8.SignatureDomainSeparation_RemoveDataCap

type State interface {
	cbor.Marshaler

	RootKey() (address.Address, error)
	VerifiedClientDataCap(address.Address) (bool, abi.StoragePower, error)
	VerifierDataCap(address.Address) (bool, abi.StoragePower, error)
	RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error)
	ForEachVerifier(func(addr address.Address, dcap abi.StoragePower) error) error
	ForEachClient(func(addr address.Address, dcap abi.StoragePower) error) error
	GetState() interface{}
}
