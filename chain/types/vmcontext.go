package types

import (
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
)

type Storage interface {
	Put(interface{}) (cid.Cid, aerrors.ActorError)
	Get(cid.Cid, interface{}) aerrors.ActorError

	GetHead() cid.Cid

	// Commit sets the new head of the actors state as long as the current
	// state matches 'oldh'
	Commit(oldh cid.Cid, newh cid.Cid) aerrors.ActorError
}

type StateTree interface {
	SetActor(addr address.Address, act *Actor) aerrors.ActorError
	GetActor(addr address.Address) (*Actor, aerrors.ActorError)
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
}
