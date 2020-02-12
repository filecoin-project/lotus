package actors

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/ipfs/go-cid"
)

var AccountCodeCid = builtin.AccountActorCodeID
var CronCodeCid = builtin.CronActorCodeID
var StoragePowerCodeCid = builtin.StoragePowerActorCodeID
var StorageMarketCodeCid = builtin.StorageMarketActorCodeID
var StorageMinerCodeCid = builtin.StorageMinerActorCodeID
var MultisigCodeCid = builtin.MultisigActorCodeID
var InitCodeCid = builtin.InitActorCodeID
var PaymentChannelCodeCid = builtin.PaymentChannelActorCodeID

var SystemAddress = builtin.SystemActorAddr
var InitAddress = builtin.InitActorAddr
var RewardActor = builtin.RewardActorAddr
var CronAddress = builtin.CronActorAddr
var StoragePowerAddress = builtin.StoragePowerActorAddr
var StorageMarketAddress = builtin.StorageMarketActorAddr
var BurntFundsAddress = builtin.BurntFundsActorAddr

/*
SystemActorAddr        = mustMakeAddress(0)
	InitActorAddr          = mustMakeAddress(1)
	RewardActorAddr        = mustMakeAddress(2)
	CronActorAddr          = mustMakeAddress(3)
	StoragePowerActorAddr  = mustMakeAddress(4)
	StorageMarketActorAddr = mustMakeAddress(5)
	// Distinguished AccountActor that is the destination of all burnt funds.
	BurntFundsActorAddr = mustMakeAddress(99)
 */

func mustIDAddress(i uint64) address.Address {
	a, err := address.NewIDAddress(i)
	if err != nil {
		panic(err) // ok
	}
	return a
}

func init() {
	BuiltInActors = map[cid.Cid]bool{
		StorageMarketCodeCid:  true,
		StoragePowerCodeCid:   true,
		StorageMinerCodeCid:   true,
		AccountCodeCid:        true,
		InitCodeCid:           true,
		MultisigCodeCid:       true,
		PaymentChannelCodeCid: true,
	}
}
