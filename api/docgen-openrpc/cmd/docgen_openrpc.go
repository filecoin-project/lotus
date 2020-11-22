package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/filecoin-project/lotus/api/apistruct"
	docgen_openrpc "github.com/filecoin-project/lotus/api/docgen-openrpc"
)

/*
main defines a small program that writes an OpenRPC document describing
a Lotus API to stdout.

If the first argument is "miner", the document will describe the StorageMiner API.
If not (no, or any other args), the document will describe the Full API.

Use:

	go run ./api/openrpc/cmd ["api/api_full.go"|"api/api_storage.go"|"api/api_worker.go"] ["FullNode"|"StorageMiner"|"WorkerAPI"]

*/

func main() {
	doc := docgen_openrpc.NewLotusOpenRPCDocument()

	switch os.Args[2] {
	case "FullNode":
		doc.RegisterReceiverName("Filecoin", &apistruct.FullNodeStruct{})
	case "StorageMiner":
		doc.RegisterReceiverName("Filecoin", &apistruct.StorageMinerStruct{})
	case "WorkerAPI":
		doc.RegisterReceiverName("Filecoin", &apistruct.WorkerStruct{})
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
