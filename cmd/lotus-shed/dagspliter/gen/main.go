package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/cmd/lotus-shed/dagspliter"
)

func main() {
	err := gen.WriteMapEncodersToFile("./cbor_gen.go", "dagspliter",
		dagspliter.Prefix{},
		dagspliter.Edge{},
		dagspliter.Box{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
