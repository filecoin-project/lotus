package builders

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/multiformats/go-varint"
)

// AddressHandle encapsulates both the ID and Robust addresses of an actor.
type AddressHandle struct {
	ID, Robust address.Address
}

func (ah AddressHandle) IDAddr() address.Address {
	return ah.ID
}

func (ah AddressHandle) RobustAddr() address.Address {
	return ah.Robust
}

func (ah AddressHandle) String() string {
	return fmt.Sprintf("AddressHandle[ID: %s, Robust: %s]", ah.ID, ah.Robust)
}

// NextActorAddress predicts the address of the next actor created by this address.
//
// Code is adapted from vm.Runtime#NewActorAddress()
func (ah *AddressHandle) NextActorAddress(nonce, numActorsCreated uint64) address.Address {
	var b bytes.Buffer
	if err := ah.Robust.MarshalCBOR(&b); err != nil {
		panic(aerrors.Fatalf("writing caller address into assert buffer: %v", err))
	}

	if err := binary.Write(&b, binary.BigEndian, nonce); err != nil {
		panic(aerrors.Fatalf("writing nonce address into assert buffer: %v", err))
	}
	if err := binary.Write(&b, binary.BigEndian, numActorsCreated); err != nil {
		panic(aerrors.Fatalf("writing callSeqNum address into assert buffer: %v", err))
	}
	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		panic(aerrors.Fatalf("create actor address: %v", err))
	}
	return addr
}

// MustNewIDAddr returns an address.Address of kind ID.
func MustNewIDAddr(id uint64) address.Address {
	addr, err := address.NewIDAddress(id)
	if err != nil {
		panic(err)
	}
	return addr
}

// MustNewSECP256K1Addr returns an address.Address of kind secp256k1.
func MustNewSECP256K1Addr(pubkey string) address.Address {
	// the pubkey of assert secp256k1 address is hashed for consistent length.
	addr, err := address.NewSecp256k1Address([]byte(pubkey))
	if err != nil {
		panic(err)
	}
	return addr
}

// MustNewBLSAddr returns an address.Address of kind bls.
func MustNewBLSAddr(seed int64) address.Address {
	buf := make([]byte, address.BlsPublicKeyBytes)
	binary.PutVarint(buf, seed)

	addr, err := address.NewBLSAddress(buf)
	if err != nil {
		panic(err)
	}
	return addr
}

// MustNewActorAddr returns an address.Address of kind actor.
func MustNewActorAddr(data string) address.Address {
	addr, err := address.NewActorAddress([]byte(data))
	if err != nil {
		panic(err)
	}
	return addr
}

// MustIDFromAddress returns the integer ID from an ID address.
func MustIDFromAddress(a address.Address) uint64 {
	if a.Protocol() != address.ID {
		panic("must be ID protocol address")
	}
	id, _, err := varint.FromUvarint(a.Payload())
	if err != nil {
		panic(err)
	}
	return id
}
