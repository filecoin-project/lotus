package actors

import (
	cbor "github.com/ipfs/go-ipld-cbor"
)

var (
	EmptyStructCBOR = []byte{0xa0}
)

func SerializeParams(i interface{}) ([]byte, error) {
	return cbor.DumpObject(i)
}
