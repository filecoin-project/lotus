package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/impl/graphsync"

)

// main func has ONE JOB
func main() {
	fmt.Print("Generating Cbor Marshal/Unmarshal...")

	if err := message.RunCborGen(); err != nil {
		fmt.Println("Failed: ")
		fmt.Println(err)
		os.Exit(1)
	}
	if err := graphsyncimpl.RunCborGen(); err != nil {
		fmt.Println("Failed: ")
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
