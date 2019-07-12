package types

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
)

type Storage interface {
	Put(interface{}) (cid.Cid, error)
	Get(cid.Cid, interface{}) error

	GetHead() cid.Cid

	// Commit sets the new head of the actors state as long as the current
	// state matches 'oldh'
	Commit(oldh cid.Cid, newh cid.Cid) error
}

type StateTree interface {
	SetActor(addr address.Address, act *Actor) error
	GetActor(addr address.Address) (*Actor, error)
}

type VMContext interface {
	Message() *Message
	Ipld() *hamt.CborIpldStore
	Send(to address.Address, method uint64, value BigInt, params []byte) ([]byte, uint8, error)
	BlockHeight() uint64
	GasUsed() BigInt
	Storage() Storage
	StateTree() (StateTree, error)
}
