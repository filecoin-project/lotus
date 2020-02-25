package types

import (
	"github.com/ipfs/go-cid"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
)

var ErrActorNotFound = init_.ErrAddressNotFound

type Actor struct {
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}
