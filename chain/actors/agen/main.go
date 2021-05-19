package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/actors/agen/generator"
)

func main() {
	if err := generator.Gen("lotus"); err != nil {
		fmt.Println(err)
	}
}
