package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(SignedStorageAsk{})
	cbor.RegisterCborType(StorageAsk{})
}

type SignedStorageAsk = storagemarket.SignedStorageAsk

type StorageAsk struct {
	// Price per GiB / Epoch
	Price BigInt

	MinPieceSize uint64
	Miner        address.Address
	Timestamp    uint64
	Expiry       uint64
	SeqNo        uint64
}
