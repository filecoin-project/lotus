package types

import (
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
)

var ErrActorNotFound = init_.ErrAddressNotFound

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
