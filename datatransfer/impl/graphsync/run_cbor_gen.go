package graphsyncimpl

import (
	"fmt"
	"os"

	cborgen "github.com/whyrusleeping/cbor-gen"
)

func RunCborGen() error {
	genName := "./impl/graphsync/cbor_gen.go"
	reName := "./impl/graphsync/cbor_gen_old.go"
	if err := os.Rename(genName, reName); err != nil {
		return fmt.Errorf("could not rename %s to %s", genName, reName)
	}
	if err := cborgen.WriteTupleEncodersToFile(
		genName,
		"graphsyncimpl",
		ExtensionDataTransferData{},
	); err != nil {
		return err
	}
	if err := os.Remove(reName); err != nil {
		return err
	}
	return nil
}
