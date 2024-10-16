package builtin

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	smoothingtypes "github.com/filecoin-project/go-state-types/builtin/v8/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/proof"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
)

var InitActorAddr = builtin.InitActorAddr
var SystemActorAddr = builtin.SystemActorAddr
var BurntFundsActorAddr = builtin.BurntFundsActorAddr
var CronActorAddr = builtin.CronActorAddr
var DatacapActorAddr = builtin.DatacapActorAddr
var EthereumAddressManagerActorAddr = builtin.EthereumAddressManagerActorAddr
var SaftAddress = makeAddress("t0122")
var ReserveAddress = makeAddress("t090")
var RootVerifierAddress = makeAddress("t080")

var (
	ExpectedLeadersPerEpoch = builtin.ExpectedLeadersPerEpoch
)

const (
	EpochDurationSeconds = builtin.EpochDurationSeconds
	EpochsInDay          = builtin.EpochsInDay
	EpochsInYear         = builtin.EpochsInYear
	SecondsInDay         = builtin.SecondsInDay
)

const (
	MethodSend        = builtin.MethodSend
	MethodConstructor = builtin.MethodConstructor
)

// These are all just type aliases across actor versions. In the future, that might change
// and we might need to do something fancier.
type SectorInfo = proof.SectorInfo
type ExtendedSectorInfo = proof.ExtendedSectorInfo
type PoStProof = proof.PoStProof
type FilterEstimate = smoothingtypes.FilterEstimate

func QAPowerForWeight(size abi.SectorSize, duration abi.ChainEpoch, verifiedWeight abi.DealWeight) abi.StoragePower {
	return minertypes.QAPowerForWeight(size, duration, verifiedWeight)
}

func ActorNameByCode(c cid.Cid) string {
	if name, version, ok := actors.GetActorMetaByCode(c); ok {
		return fmt.Sprintf("fil/%d/%s", version, name)
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

	return false
}

func IsStorageMinerActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == manifest.MinerKey
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

	return false
}

func IsMultisigActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == manifest.MultisigKey
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

	return false
}

func IsPlaceholderActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == manifest.PlaceholderKey
	}

	return false
}

func IsEvmActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == manifest.EvmKey
	}

	return false
}

func IsEthAccountActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == manifest.EthAccountKey
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
