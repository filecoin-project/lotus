package actors

import (
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var (
	EmptyStructCBOR = []byte{0xa0}
)

func SerializeParams(i interface{}) ([]byte, aerrors.ActorError) {
	dump, err := cbor.DumpObject(i)
	if err != nil {
		return nil, aerrors.Absorb(err, 1, "failed to encode parameter")
	}
	return dump, nil
}
