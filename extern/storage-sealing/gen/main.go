package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
)

func main() {
	err := gen.WriteMapEncodersToFile("./cbor_gen.go", "sealing",
		sealing.Piece{},
		sealing.SectorInfo{},
		sealing.Log{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
