package message

import (
	"fmt"
	"os"

	cborgen "github.com/whyrusleeping/cbor-gen"
)

func RunCborGen() error {
	genName := "./message/cbor_gen.go"
	reName := "./message/cbor_gen_old.go"
	if err := os.Rename(genName, reName); err != nil {
		return fmt.Errorf("could not rename %s to %s", genName, reName)
	}
	if err := cborgen.WriteTupleEncodersToFile(
		genName,
		"message",
		transferMessage{},
		transferRequest{},
		transferResponse{},
	); err != nil {
		return err
	}
	if err := os.Remove(reName); err != nil {
		return err
	}
	return nil
}
