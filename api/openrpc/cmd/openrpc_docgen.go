package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/api/openrpc"
)

func main() {
	commonPermStruct := &apistruct.CommonStruct{}
	fullStruct := &apistruct.FullNodeStruct{}

	doc := openrpc.NewLotusOpenRPCDocument()

	doc.RegisterReceiverName("common", commonPermStruct)
	doc.RegisterReceiverName("full", fullStruct)

	out, err := doc.Discover()
	if err != nil {
		log.Fatalln(err)
	}

	jsonOut, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(jsonOut))

	// fmt.Println("---- (Comments?) Out:")
	// // out2, groupDocs := parseApiASTInfo()
	// out2, _ := parseApiASTInfo()
	// out2JSON, _ := json.MarshalIndent(out2, "", "    ")
	// fmt.Println(string(out2JSON))
	// fmt.Println("---- (Group Comments?):")
	// groupDocsJSON, _ := json.MarshalIndent(groupDocs, "", "    ")
	// fmt.Println(string(groupDocsJSON))
}
