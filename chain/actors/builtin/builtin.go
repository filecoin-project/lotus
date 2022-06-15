package builtin

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner8 "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	smoothingtypes "github.com/filecoin-project/go-state-types/builtin/v8/util/smoothing"
	"github.com/filecoin-project/go-state-types/proof"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
)

var SystemActorAddr = builtin.SystemActorAddr
var BurntFundsActorAddr = builtin.BurntFundsActorAddr
var CronActorAddr = builtin.CronActorAddr
var SaftAddress = makeAddress("t0122")
var ReserveAddress = makeAddress("t090")
var RootVerifierAddress = makeAddress("t080")

var (
	ExpectedLeadersPerEpoch = builtin.ExpectedLeadersPerEpoch
)

const (
	EpochDurationSeconds = builtin.EpochDurationSeconds
	EpochsInDay          = builtin.EpochsInDay
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

func QAPowerForWeight(size abi.SectorSize, duration abi.ChainEpoch, dealWeight, verifiedWeight abi.DealWeight) abi.StoragePower {
	return miner8.QAPowerForWeight(size, duration, dealWeight, verifiedWeight)
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

	return false
}

func GetAccountActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.AccountKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.AccountActorCodeID, nil

	case actors.Version2:
		return builtin2.AccountActorCodeID, nil

	case actors.Version3:
		return builtin3.AccountActorCodeID, nil

	case actors.Version4:
		return builtin4.AccountActorCodeID, nil

	case actors.Version5:
		return builtin5.AccountActorCodeID, nil

	case actors.Version6:
		return builtin6.AccountActorCodeID, nil

	case actors.Version7:
		return builtin7.AccountActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
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

func GetCronActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.CronKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.CronActorCodeID, nil

	case actors.Version2:
		return builtin2.CronActorCodeID, nil

	case actors.Version3:
		return builtin3.CronActorCodeID, nil

	case actors.Version4:
		return builtin4.CronActorCodeID, nil

	case actors.Version5:
		return builtin5.CronActorCodeID, nil

	case actors.Version6:
		return builtin6.CronActorCodeID, nil

	case actors.Version7:
		return builtin7.CronActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetInitActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.InitKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.InitActorCodeID, nil

	case actors.Version2:
		return builtin2.InitActorCodeID, nil

	case actors.Version3:
		return builtin3.InitActorCodeID, nil

	case actors.Version4:
		return builtin4.InitActorCodeID, nil

	case actors.Version5:
		return builtin5.InitActorCodeID, nil

	case actors.Version6:
		return builtin6.InitActorCodeID, nil

	case actors.Version7:
		return builtin7.InitActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetMarketActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.MarketKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.StorageMarketActorCodeID, nil

	case actors.Version2:
		return builtin2.StorageMarketActorCodeID, nil

	case actors.Version3:
		return builtin3.StorageMarketActorCodeID, nil

	case actors.Version4:
		return builtin4.StorageMarketActorCodeID, nil

	case actors.Version5:
		return builtin5.StorageMarketActorCodeID, nil

	case actors.Version6:
		return builtin6.StorageMarketActorCodeID, nil

	case actors.Version7:
		return builtin7.StorageMarketActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetMinerActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.MinerKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.StorageMinerActorCodeID, nil

	case actors.Version2:
		return builtin2.StorageMinerActorCodeID, nil

	case actors.Version3:
		return builtin3.StorageMinerActorCodeID, nil

	case actors.Version4:
		return builtin4.StorageMinerActorCodeID, nil

	case actors.Version5:
		return builtin5.StorageMinerActorCodeID, nil

	case actors.Version6:
		return builtin6.StorageMinerActorCodeID, nil

	case actors.Version7:
		return builtin7.StorageMinerActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func IsStorageMinerActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == actors.MinerKey
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

func GetMultisigActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.MultisigKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.MultisigActorCodeID, nil

	case actors.Version2:
		return builtin2.MultisigActorCodeID, nil

	case actors.Version3:
		return builtin3.MultisigActorCodeID, nil

	case actors.Version4:
		return builtin4.MultisigActorCodeID, nil

	case actors.Version5:
		return builtin5.MultisigActorCodeID, nil

	case actors.Version6:
		return builtin6.MultisigActorCodeID, nil

	case actors.Version7:
		return builtin7.MultisigActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func IsMultisigActor(c cid.Cid) bool {
	name, _, ok := actors.GetActorMetaByCode(c)
	if ok {
		return name == actors.MultisigKey
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

func GetPaymentChannelActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.PaychKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.PaymentChannelActorCodeID, nil

	case actors.Version2:
		return builtin2.PaymentChannelActorCodeID, nil

	case actors.Version3:
		return builtin3.PaymentChannelActorCodeID, nil

	case actors.Version4:
		return builtin4.PaymentChannelActorCodeID, nil

	case actors.Version5:
		return builtin5.PaymentChannelActorCodeID, nil

	case actors.Version6:
		return builtin6.PaymentChannelActorCodeID, nil

	case actors.Version7:
		return builtin7.PaymentChannelActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
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

func GetPowerActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.PowerKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.StoragePowerActorCodeID, nil

	case actors.Version2:
		return builtin2.StoragePowerActorCodeID, nil

	case actors.Version3:
		return builtin3.StoragePowerActorCodeID, nil

	case actors.Version4:
		return builtin4.StoragePowerActorCodeID, nil

	case actors.Version5:
		return builtin5.StoragePowerActorCodeID, nil

	case actors.Version6:
		return builtin6.StoragePowerActorCodeID, nil

	case actors.Version7:
		return builtin7.StoragePowerActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetRewardActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.RewardKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.RewardActorCodeID, nil

	case actors.Version2:
		return builtin2.RewardActorCodeID, nil

	case actors.Version3:
		return builtin3.RewardActorCodeID, nil

	case actors.Version4:
		return builtin4.RewardActorCodeID, nil

	case actors.Version5:
		return builtin5.RewardActorCodeID, nil

	case actors.Version6:
		return builtin6.RewardActorCodeID, nil

	case actors.Version7:
		return builtin7.RewardActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetSystemActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.SystemKey); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.SystemActorCodeID, nil

	case actors.Version2:
		return builtin2.SystemActorCodeID, nil

	case actors.Version3:
		return builtin3.SystemActorCodeID, nil

	case actors.Version4:
		return builtin4.SystemActorCodeID, nil

	case actors.Version5:
		return builtin5.SystemActorCodeID, nil

	case actors.Version6:
		return builtin6.SystemActorCodeID, nil

	case actors.Version7:
		return builtin7.SystemActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func GetVerifregActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, actors.VerifregKey); ok {
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

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

func makeAddress(addr string) address.Address {
	ret, err := address.NewFromString(addr)
	if err != nil {
		panic(err)
	}

	return ret
}
