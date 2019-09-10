package actors

import (
	"bytes"

	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	cbg "github.com/whyrusleeping/cbor-gen"
)

var (
	EmptyStructCBOR = []byte{0xa0}
)

func SerializeParams(i cbg.CBORMarshaler) ([]byte, aerrors.ActorError) {
	buf := new(bytes.Buffer)
	if err := i.MarshalCBOR(buf); err != nil {
		// TODO: shouldnt this be a fatal error?
		return nil, aerrors.Absorb(err, 1, "failed to encode parameter")
	}
	return buf.Bytes(), nil
}
