package types

import (
	"errors"

	"github.com/ipfs/go-cid"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
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
	return a.Code == builtin0.AccountActorCodeID
}

func (a *Actor) IsStorageMinerActor() bool {
	return a.Code == builtin0.StorageMinerActorCodeID
}

func (a *Actor) IsMultisigActor() bool {
	return a.Code == builtin0.MultisigActorCodeID
}

func (a *Actor) IsPaymentChannelActor() bool {
	return a.Code == builtin0.PaymentChannelActorCodeID
}
