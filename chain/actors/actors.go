package actors

import (
	"github.com/filecoin-project/lotus/chain/address"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

var AccountCodeCid cid.Cid
var StoragePowerCodeCid cid.Cid
var StorageMarketCodeCid cid.Cid
var StorageMinerCodeCid cid.Cid
var MultisigCodeCid cid.Cid
var InitCodeCid cid.Cid
var PaymentChannelCodeCid cid.Cid

var InitAddress = mustIDAddress(0)
var NetworkAddress = mustIDAddress(1)
var StoragePowerAddress = mustIDAddress(2)
var StorageMarketAddress = mustIDAddress(3) // TODO: missing from spec
var BurntFundsAddress = mustIDAddress(99)

func mustIDAddress(i uint64) address.Address {
	a, err := address.NewIDAddress(i)
	if err != nil {
		panic(err)
	}
	return a
}

func init() {
	pref := cid.NewPrefixV1(cid.Raw, mh.ID)
	mustSum := func(s string) cid.Cid {
		c, err := pref.Sum([]byte(s))
		if err != nil {
			panic(err)
		}
		return c
	}

	AccountCodeCid = mustSum("filecoin/1.0/AccountActor")
	StoragePowerCodeCid = mustSum("filecoin/1.0/StoragePowerActor")
	StorageMarketCodeCid = mustSum("filecoin/1.0/StorageMarketActor")
	StorageMinerCodeCid = mustSum("filecoin/1.0/StorageMinerActor")
	MultisigCodeCid = mustSum("filecoin/1.0/MultisigActor")
	InitCodeCid = mustSum("filecoin/1.0/InitActor")
	PaymentChannelCodeCid = mustSum("filecoin/1.0/PaymentChannelActor")
}
