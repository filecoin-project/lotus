package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/api/openrpc"
)

/*
main defines a small program that writes an OpenRPC document describing
a Lotus API to stdout.

If the first argument is "miner", the document will describe the StorageMiner API.
If not (no, or any other args), the document will describe the Full API.

Use:

	go run ./api/openrpc/cmd [|miner]

*/

func main() {
	var full, miner bool
	full = true
	if len(os.Args) > 1 && os.Args[1] == "miner" {
		log.Println("Running generation for Miner API")
		miner = true
		full = false
	}

	commonPermStruct := &apistruct.CommonStruct{}
	fullStruct := &apistruct.FullNodeStruct{}
	minerStruct := &apistruct.StorageMinerStruct{}

	doc := openrpc.NewLotusOpenRPCDocument()

	if full {
		doc.RegisterReceiverName("Filecoin", commonPermStruct)
		doc.RegisterReceiverName("Filecoin", fullStruct)
	} else if miner {
		doc.RegisterReceiverName("Filecoin", minerStruct)
	}

	out, err := doc.Discover()
	if err != nil {
		log.Fatalln(err)
	}

	jsonOut, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(jsonOut))
}
