package types

import (
	"context"

	"github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/address"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
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
	Ipld() *hamt.CborIpldStore
	Send(to address.Address, method uint64, value BigInt, params []byte) ([]byte, aerrors.ActorError)
	BlockHeight() uint64
	GasUsed() BigInt
	Storage() Storage
	StateTree() (StateTree, aerrors.ActorError)
	VerifySignature(sig *Signature, from address.Address, data []byte) aerrors.ActorError
	ChargeGas(uint64) aerrors.ActorError
	GetRandomness(height uint64) ([]byte, aerrors.ActorError)
	GetBalance(address.Address) (BigInt, aerrors.ActorError)

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

func WrapStorage(s Storage) amt.Blocks {
	return &storageWrapper{s}
}
