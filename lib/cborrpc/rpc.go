package cborrpc

import (
	"io"

	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
)

const MessageSizeLimit = 1 << 20

func WriteCborRPC(w io.Writer, obj interface{}) error {
	if m, ok := obj.(cbg.CBORMarshaler); ok {
		return m.MarshalCBOR(w)
	}
	data, err := cbor.DumpObject(obj)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func ReadCborRPC(r io.Reader, out interface{}) error {
	if um, ok := out.(cbg.CBORUnmarshaler); ok {
		return um.UnmarshalCBOR(r)
	}
	return cbor.DecodeReader(r, out)
}
