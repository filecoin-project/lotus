package chain

import (
	"encoding/binary"

	addr "github.com/filecoin-project/go-address"
	"github.com/multiformats/go-varint"
)

// If you use this method while writing a test you are more than likely doing something wrong.
func MustNewIDAddr(id uint64) addr.Address {
	address, err := addr.NewIDAddress(id)
	if err != nil {
		panic(err)
	}
	return address
}

func MustNewSECP256K1Addr(pubkey string) addr.Address {
	// the pubkey of a secp256k1 address is hashed for consistent length.
	address, err := addr.NewSecp256k1Address([]byte(pubkey))
	if err != nil {
		panic(err)
	}
	return address
}

func MustNewBLSAddr(seed int64) addr.Address {
	buf := make([]byte, addr.BlsPublicKeyBytes)
	binary.PutVarint(buf, seed)

	address, err := addr.NewBLSAddress(buf)
	if err != nil {
		panic(err)
	}
	return address
}

func MustNewActorAddr(data string) addr.Address {
	address, err := addr.NewActorAddress([]byte(data))
	if err != nil {
		panic(err)
	}
	return address
}

func MustIDFromAddress(a addr.Address) uint64 {
	if a.Protocol() != addr.ID {
		panic("must be ID protocol address")
	}
	id, _, err := varint.FromUvarint(a.Payload())
	if err != nil {
		panic(err)
	}
	return id
}
