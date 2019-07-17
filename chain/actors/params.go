package actors

import (
	cbor "github.com/ipfs/go-ipld-cbor"
)

func SerializeParams(i interface{}) ([]byte, error) {
	return cbor.DumpObject(i)
}
