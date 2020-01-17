package actors

import (
	"github.com/filecoin-project/go-address"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

var AccountCodeCid cid.Cid
var CronCodeCid cid.Cid
var StoragePowerCodeCid cid.Cid
var StorageMarketCodeCid cid.Cid
var StorageMinerCodeCid cid.Cid
var StorageMiner2CodeCid cid.Cid
var MultisigCodeCid cid.Cid
var InitCodeCid cid.Cid
var PaymentChannelCodeCid cid.Cid

var InitAddress = mustIDAddress(0)
var NetworkAddress = mustIDAddress(1)
var StoragePowerAddress = mustIDAddress(2)
var StorageMarketAddress = mustIDAddress(3) // TODO: missing from spec
var CronAddress = mustIDAddress(4)
var BurntFundsAddress = mustIDAddress(99)

func mustIDAddress(i uint64) address.Address {
	a, err := address.NewIDAddress(i)
	if err != nil {
		panic(err) // ok
	}
	return a
}

func init() {
	pref := cid.NewPrefixV1(cid.Raw, mh.IDENTITY)
	mustSum := func(s string) cid.Cid {
		c, err := pref.Sum([]byte(s))
		if err != nil {
			panic(err) // ok
		}
		return c
	}

	AccountCodeCid = mustSum("fil/1/account") // TODO: spec
	CronCodeCid = mustSum("fil/1/cron")
	StoragePowerCodeCid = mustSum("fil/1/power")
	StorageMarketCodeCid = mustSum("fil/1/market")
	StorageMinerCodeCid = mustSum("fil/1/miner")
	StorageMiner2CodeCid = mustSum("fil/1/miner/2")
	MultisigCodeCid = mustSum("fil/1/multisig")
	InitCodeCid = mustSum("fil/1/init")
	PaymentChannelCodeCid = mustSum("fil/1/paych")
}
