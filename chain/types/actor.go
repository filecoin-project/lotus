package types

import (
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var ErrActorNotFound = fmt.Errorf("actor not found")

func init() {
	cbor.RegisterCborType(Actor{})
}

type Actor struct {
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}
