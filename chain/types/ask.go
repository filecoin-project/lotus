package types

import (
	"github.com/filecoin-project/go-address"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(SignedStorageAsk{})
	cbor.RegisterCborType(StorageAsk{})
}

type SignedStorageAsk struct {
	Ask       *StorageAsk
	Signature *Signature
}

type StorageAsk struct {
	// Price per GiB / Epoch
	Price BigInt

	MinPieceSize uint64
	Miner        address.Address
	Timestamp    uint64
	Expiry       uint64
	SeqNo        uint64
}
