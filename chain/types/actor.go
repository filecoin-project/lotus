package types

import (
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
)

var ErrActorNotFound = errors.New("actor not found")

type Actor struct {
	// Identifies the type of actor (string coded as a CID), see `chain/actors/actors.go`.
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
	// Address is the predictable address of this actor.
	Address *address.Address
}
