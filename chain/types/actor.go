package types

import (
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/specs-actors/actors/builtin"
)

var ErrActorNotFound = errors.New("actor not found")

type Actor struct {
	// Identifies the type of actor (string coded as a CID), see `chain/actors/actors.go`.
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}

func (a *Actor) IsAccountActor() bool {
	return a.Code == builtin.AccountActorCodeID
}
