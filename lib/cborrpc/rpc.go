package cborrpc

import (
	"encoding/hex"
	"io"

	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	cbg "github.com/whyrusleeping/cbor-gen"
)

var log = logging.Logger("cborrrpc")

const Debug = false

func init() {
	if Debug {
		log.Warn("CBOR-RPC Debugging enabled")
	}
}

func WriteCborRPC(w io.Writer, obj interface{}) error {
	if m, ok := obj.(cbg.CBORMarshaler); ok {
		// TODO: impl debug
		return m.MarshalCBOR(w)
	}
	data, err := cbor.DumpObject(obj)
	if err != nil {
		return err
	}

	if Debug {
		log.Infof("> %s", hex.EncodeToString(data))
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
