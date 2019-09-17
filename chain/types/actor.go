package types

import (
	"fmt"

	"github.com/ipfs/go-cid"
)

var ErrActorNotFound = fmt.Errorf("actor not found")

type Actor struct {
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}
