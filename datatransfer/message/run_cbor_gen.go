package message

import (
	cborgen "github.com/whyrusleeping/cbor-gen"
)

func RunCborGen() error {
	return cborgen.WriteTupleEncodersToFile(
		"./message/cbor_gen.go",
		"message",
		transferMessage{},
		transferRequest{},
		transferResponse{},
	)
}
