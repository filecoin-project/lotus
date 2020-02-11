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

var SystemAddress = mustIDAddress(0)
var InitAddress = mustIDAddress(1)
var RewardActor = mustIDAddress(2)
var CronAddress = mustIDAddress(3)
var StoragePowerAddress = mustIDAddress(4)
var StorageMarketAddress = mustIDAddress(5)

var NetworkAddress = mustIDAddress(17) // TODO: needs to be removed in favor of reward actor

var BurntFundsAddress = mustIDAddress(99)

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
