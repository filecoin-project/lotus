package builtin

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	
		builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
		smoothing0 "github.com/filecoin-project/specs-actors/actors/util/smoothing"
	
		builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
		smoothing2 "github.com/filecoin-project/specs-actors/v2/actors/util/smoothing"
	
		builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
		smoothing3 "github.com/filecoin-project/specs-actors/v3/actors/util/smoothing"
	
		builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
		smoothing4 "github.com/filecoin-project/specs-actors/v4/actors/util/smoothing"
	
		builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
		smoothing5 "github.com/filecoin-project/specs-actors/v5/actors/util/smoothing"
	
		builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
		smoothing6 "github.com/filecoin-project/specs-actors/v6/actors/util/smoothing"
	
		builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
		smoothing7 "github.com/filecoin-project/specs-actors/v7/actors/util/smoothing"
	
		builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"
		smoothing8 "github.com/filecoin-project/specs-actors/v8/actors/util/smoothing"
	

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"

	miner8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/miner"
	proof8 "github.com/filecoin-project/specs-actors/v8/actors/runtime/proof"
)

var SystemActorAddr = builtin8.SystemActorAddr
var BurntFundsActorAddr = builtin8.BurntFundsActorAddr
var CronActorAddr = builtin8.CronActorAddr
var SaftAddress = makeAddress("t0122")
var ReserveAddress = makeAddress("t090")
var RootVerifierAddress = makeAddress("t080")

var (
	ExpectedLeadersPerEpoch = builtin8.ExpectedLeadersPerEpoch
)

const (
	EpochDurationSeconds = builtin8.EpochDurationSeconds
	EpochsInDay          = builtin8.EpochsInDay
	SecondsInDay         = builtin8.SecondsInDay
)

const (
	MethodSend        = builtin8.MethodSend
	MethodConstructor = builtin8.MethodConstructor
)

// These are all just type aliases across actor versions. In the future, that might change
// and we might need to do something fancier.
type SectorInfo = proof8.SectorInfo
type ExtendedSectorInfo = proof8.ExtendedSectorInfo
type PoStProof = proof8.PoStProof
type FilterEstimate = smoothing0.FilterEstimate

func QAPowerForWeight(size abi.SectorSize, duration abi.ChainEpoch, dealWeight, verifiedWeight abi.DealWeight) abi.StoragePower {
	return miner8.QAPowerForWeight(size, duration, dealWeight, verifiedWeight)
}


	func FromV0FilterEstimate(v0 smoothing0.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v0) //nolint:unconvert
	
	}

	func FromV2FilterEstimate(v2 smoothing2.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v2)
	
	}

	func FromV3FilterEstimate(v3 smoothing3.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v3)
	
	}

	func FromV4FilterEstimate(v4 smoothing4.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v4)
	
	}

	func FromV5FilterEstimate(v5 smoothing5.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v5)
	
	}

	func FromV6FilterEstimate(v6 smoothing6.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v6)
	
	}

	func FromV7FilterEstimate(v7 smoothing7.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v7)
	
	}

	func FromV8FilterEstimate(v8 smoothing8.FilterEstimate) FilterEstimate {
	
		return (FilterEstimate)(v8)
	
	}


type ActorStateLoader func(store adt.Store, root cid.Cid) (cbor.Marshaler, error)

var ActorStateLoaders = make(map[cid.Cid]ActorStateLoader)

func RegisterActorState(code cid.Cid, loader ActorStateLoader) {
	ActorStateLoaders[code] = loader
}

func Load(store adt.Store, act *types.Actor) (cbor.Marshaler, error) {
	loader, found := ActorStateLoaders[act.Code]
	if !found {
		return nil, xerrors.Errorf("unknown actor code %s", act.Code)
	}
	return loader(store, act.Head)
}

func ActorNameByCode(c cid.Cid) string {
	name, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return name
	}

	switch {
		
			case builtin0.IsBuiltinActor(c):
			return builtin0.ActorNameByCode(c)
		
			case builtin2.IsBuiltinActor(c):
			return builtin2.ActorNameByCode(c)
		
			case builtin3.IsBuiltinActor(c):
			return builtin3.ActorNameByCode(c)
		
			case builtin4.IsBuiltinActor(c):
			return builtin4.ActorNameByCode(c)
		
			case builtin5.IsBuiltinActor(c):
			return builtin5.ActorNameByCode(c)
		
			case builtin6.IsBuiltinActor(c):
			return builtin6.ActorNameByCode(c)
		
			case builtin7.IsBuiltinActor(c):
			return builtin7.ActorNameByCode(c)
		
			case builtin8.IsBuiltinActor(c):
			return builtin8.ActorNameByCode(c)
		
	default:
		return "<unknown>"
	}
}

func IsBuiltinActor(c cid.Cid) bool {
	_, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return true
	}

	
		if builtin0.IsBuiltinActor(c) {
			return true
		}
	
		if builtin2.IsBuiltinActor(c) {
			return true
		}
	
		if builtin3.IsBuiltinActor(c) {
			return true
		}
	
		if builtin4.IsBuiltinActor(c) {
			return true
		}
	
		if builtin5.IsBuiltinActor(c) {
			return true
		}
	
		if builtin6.IsBuiltinActor(c) {
			return true
		}
	
		if builtin7.IsBuiltinActor(c) {
			return true
		}
	
		if builtin8.IsBuiltinActor(c) {
			return true
		}
	
	return false
}

func IsAccountActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return name == "account"
	}

	
		if c == builtin0.AccountActorCodeID {
			return true
		}
	
		if c == builtin2.AccountActorCodeID {
			return true
		}
	
		if c == builtin3.AccountActorCodeID {
			return true
		}
	
		if c == builtin4.AccountActorCodeID {
			return true
		}
	
		if c == builtin5.AccountActorCodeID {
			return true
		}
	
		if c == builtin6.AccountActorCodeID {
			return true
		}
	
		if c == builtin7.AccountActorCodeID {
			return true
		}
	
		if c == builtin8.AccountActorCodeID {
			return true
		}
	
	return false
}

func IsStorageMinerActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return name == "storageminer"
	}

	
		if c == builtin0.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin2.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin3.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin4.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin5.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin6.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin7.StorageMinerActorCodeID {
			return true
		}
	
		if c == builtin8.StorageMinerActorCodeID {
			return true
		}
	
	return false
}

func IsMultisigActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return name == "multisig"
	}

	
		if c == builtin0.MultisigActorCodeID {
			return true
		}
	
		if c == builtin2.MultisigActorCodeID {
			return true
		}
	
		if c == builtin3.MultisigActorCodeID {
			return true
		}
	
		if c == builtin4.MultisigActorCodeID {
			return true
		}
	
		if c == builtin5.MultisigActorCodeID {
			return true
		}
	
		if c == builtin6.MultisigActorCodeID {
			return true
		}
	
		if c == builtin7.MultisigActorCodeID {
			return true
		}
	
		if c == builtin8.MultisigActorCodeID {
			return true
		}
	
	return false
}

func IsPaymentChannelActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
    if ok {
    	return name == "paymentchannel"
	}

	
		if c == builtin0.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin2.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin3.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin4.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin5.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin6.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin7.PaymentChannelActorCodeID {
			return true
		}
	
		if c == builtin8.PaymentChannelActorCodeID {
			return true
		}
	
	return false
}

func makeAddress(addr string) address.Address {
	ret, err := address.NewFromString(addr)
	if err != nil {
		panic(err)
	}

	return ret
}
