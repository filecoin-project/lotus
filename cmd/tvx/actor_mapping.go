package main

import (
	"reflect"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

var ActorMethodTable = make(map[string][]string, 64)

var Actors = map[cid.Cid]interface{}{
	builtin.InitActorCodeID:             builtin.MethodsInit,
	builtin.CronActorCodeID:             builtin.MethodsCron,
	builtin.AccountActorCodeID:          builtin.MethodsAccount,
	builtin.StoragePowerActorCodeID:     builtin.MethodsPower,
	builtin.StorageMinerActorCodeID:     builtin.MethodsMiner,
	builtin.StorageMarketActorCodeID:    builtin.MethodsMarket,
	builtin.PaymentChannelActorCodeID:   builtin.MethodsPaych,
	builtin.MultisigActorCodeID:         builtin.MethodsMultisig,
	builtin.RewardActorCodeID:           builtin.MethodsReward,
	builtin.VerifiedRegistryActorCodeID: builtin.MethodsVerifiedRegistry,
}

func init() {
	for code, methods := range Actors {
		cmh, err := multihash.Decode(code.Hash()) // identity hash.
		if err != nil {
			panic(err)
		}

		var (
			aname = string(cmh.Digest)
			rt    = reflect.TypeOf(methods)
			nf    = rt.NumField()
		)

		ActorMethodTable[aname] = append(ActorMethodTable[aname], "Send")
		for i := 0; i < nf; i++ {
			ActorMethodTable[aname] = append(ActorMethodTable[aname], rt.Field(i).Name)
		}
	}
}
