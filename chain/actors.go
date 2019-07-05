package chain

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-lotus/chain/address"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
)

func init() {
	cbor.RegisterCborType(InitActorState{})
	cbor.RegisterCborType(AccountActorState{})
}

var AccountActorCodeCid cid.Cid
var StorageMarketActorCodeCid cid.Cid
var StorageMinerCodeCid cid.Cid
var MultisigActorCodeCid cid.Cid
var InitActorCodeCid cid.Cid

var InitActorAddress = mustIDAddress(0)
var NetworkAddress = mustIDAddress(1)
var StorageMarketAddress = mustIDAddress(2)

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

	AccountActorCodeCid = mustSum("account")
	StorageMarketActorCodeCid = mustSum("smarket")
	StorageMinerCodeCid = mustSum("sminer")
	MultisigActorCodeCid = mustSum("multisig")
	InitActorCodeCid = mustSum("init")
}

type VMActor struct {
}

type InitActorState struct {
	AddressMap cid.Cid

	NextID uint64
}

func (ias *InitActorState) AddActor(vmctx *VMContext, addr address.Address) (address.Address, error) {
	nid := ias.NextID
	ias.NextID++

	amap, err := hamt.LoadNode(context.TODO(), vmctx.Ipld(), ias.AddressMap)
	if err != nil {
		return address.Undef, err
	}

	if err := amap.Set(context.TODO(), string(addr.Bytes()), nid); err != nil {
		return address.Undef, err
	}

	if err := amap.Flush(context.TODO()); err != nil {
		return address.Undef, err
	}

	ncid, err := vmctx.Ipld().Put(context.TODO(), amap)
	if err != nil {
		return address.Undef, err
	}
	ias.AddressMap = ncid

	return address.NewIDAddress(nid)
}

func (ias *InitActorState) Lookup(cst *hamt.CborIpldStore, addr address.Address) (address.Address, error) {
	amap, err := hamt.LoadNode(context.TODO(), cst, ias.AddressMap)
	if err != nil {
		return address.Undef, err
	}

	val, err := amap.Find(context.TODO(), string(addr.Bytes()))
	if err != nil {
		return address.Undef, err
	}

	ival, ok := val.(uint64)
	if !ok {
		return address.Undef, fmt.Errorf("invalid value in init actor state, expected uint64, got %T", val)
	}

	return address.NewIDAddress(ival)
}

type AccountActorState struct {
	Address address.Address
}
