package types

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type Storage interface {
	Put(cbg.CBORMarshaler) (cid.Cid, aerrors.ActorError)
	Get(cid.Cid, cbg.CBORUnmarshaler) aerrors.ActorError

	GetHead() cid.Cid

	// Commit sets the new head of the actors state as long as the current
	// state matches 'oldh'
	Commit(oldh cid.Cid, newh cid.Cid) aerrors.ActorError
}

type StateTree interface {
	SetActor(addr address.Address, act *Actor) error
	GetActor(addr address.Address) (*Actor, error)
}

type VMContext interface {
	Message() *Message
	Origin() address.Address
	Ipld() cbor.IpldStore
	Send(to address.Address, method abi.MethodNum, value BigInt, params []byte) ([]byte, aerrors.ActorError)
	BlockHeight() abi.ChainEpoch
	GasUsed() BigInt
	Storage() Storage
	StateTree() (StateTree, aerrors.ActorError)
	ActorCodeCID(address.Address) (cid.Cid, error)
	LookupID(address.Address) (address.Address, error)
	VerifySignature(sig *crypto.Signature, from address.Address, data []byte) aerrors.ActorError
	ChargeGas(uint64) aerrors.ActorError
	GetRandomness(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, aerrors.ActorError)
	GetBalance(address.Address) (BigInt, aerrors.ActorError)
	Sys() runtime.Syscalls

	Context() context.Context
}

type storageWrapper struct {
	s Storage
}

func (sw *storageWrapper) Put(i cbg.CBORMarshaler) (cid.Cid, error) {
	c, err := sw.s.Put(i)
	if err != nil {
		return cid.Undef, err
	}

	return c, nil
}

func (sw *storageWrapper) Get(c cid.Cid, out cbg.CBORUnmarshaler) error {
	if err := sw.s.Get(c, out); err != nil {
		return err
	}

	return nil
}
