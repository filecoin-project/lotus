package main

import (
	"fmt"
	"os"

	retrievalimpl "github.com/filecoin-project/lotus/retrieval/impl"
)

// main func has ONE JOB
func main() {
	fmt.Print("Generating Cbor Marshal/Unmarshal...")

	if err := retrievalimpl.RunCborGen(); err != nil {
		fmt.Println("Failed: ")
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
