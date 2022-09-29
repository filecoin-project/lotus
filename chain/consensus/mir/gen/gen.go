package main

import (
	"github.com/filecoin-project/lotus/chain/consensus/mir"
	gen "github.com/whyrusleeping/cbor-gen"
)

func main() {
	if err := gen.WriteTupleEncodersToFile("./cbor_gen.go", "hierarchical",
		mir.Validator{},
		mir.ValidatorSet{},
	); err != nil {
		panic(err)
	}
}
