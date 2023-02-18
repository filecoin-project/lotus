package types

import (
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
)

var ErrActorNotFound = errors.New("actor not found")

// Actor State for state tree version up to 4
type ActorV4 struct {
	// Identifies the type of actor (string coded as a CID), see `chain/actors/actors.go`.
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}

// Actor State for state tree version 5
type ActorV5 struct {
	// Identifies the type of actor (string coded as a CID), see `chain/actors/actors.go`.
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
	// Deterministic Address.
	Address *address.Address
}

type Actor = ActorV5

func AsActorV4(a *ActorV5) *ActorV4 {
	return &ActorV4{
		Code:    a.Code,
		Head:    a.Head,
		Nonce:   a.Nonce,
		Balance: a.Balance,
	}
}

func AsActorV5(a *ActorV4) *ActorV5 {
	return &ActorV5{
		Code:    a.Code,
		Head:    a.Head,
		Nonce:   a.Nonce,
		Balance: a.Balance,
	}
}
