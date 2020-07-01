package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/filecoin-project/specs-actors/actors/builtin"
)

func main() {
	if _, err := os.Stat("code.json"); err != nil {
		panic(err) // note: must run in lotuspond/front/src/chain
	}

	names := map[string]string{
		"system":   "fil/1/system",
		"init":     "fil/1/init",
		"cron":     "fil/1/cron",
		"account":  "fil/1/account",
		"power":    "fil/1/storagepower",
		"miner":    "fil/1/storageminer",
		"market":   "fil/1/storagemarket",
		"paych":    "fil/1/paymentchannel",
		"multisig": "fil/1/multisig",
		"reward":   "fil/1/reward",
		"verifreg": "fil/1/verifiedregistry",
	}

	{
		b, err := json.MarshalIndent(names, "", "  ")
		if err != nil {
			panic(err)
		}

		if err := ioutil.WriteFile("code.json", b, 0664); err != nil {
			panic(err)
		}
	}

	methods := map[cid.Cid]interface{}{
		// builtin.SystemActorCodeID:        builtin.MethodsSystem - apparently it doesn't have methods
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

	out := map[string][]string{}
	for c, methods := range methods {
		cmh, err := multihash.Decode(c.Hash())
		if err != nil {
			panic(err)
		}

		rt := reflect.TypeOf(methods)
		nf := rt.NumField()

		out[string(cmh.Digest)] = append(out[string(cmh.Digest)], "Send")
		for i := 0; i < nf; i++ {
			out[string(cmh.Digest)] = append(out[string(cmh.Digest)], rt.Field(i).Name)
		}
	}

	{
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			panic(err)
		}

		if err := ioutil.WriteFile("methods.json", b, 0664); err != nil {
			panic(err)
		}
	}
}
