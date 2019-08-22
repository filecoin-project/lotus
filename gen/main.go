package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/types"
	gen "github.com/whyrusleeping/cbor-gen"
)

func main() {
	{
		fi, err := os.Create("./chain/types/cbor_gen.go")
		if err != nil {
			fmt.Println("failed to open file: ", err)
			os.Exit(1)
		}
		defer fi.Close()

		if err := gen.PrintHeaderAndUtilityMethods(fi, "types"); err != nil {
			fmt.Println("failed to write header: ", err)
			os.Exit(1)
		}

		types := []interface{}{
			types.BlockHeader{},
			types.Ticket{},
			types.Message{},
			types.SignedMessage{},
			types.MsgMeta{},
		}

		for _, t := range types {
			if err := gen.GenTupleEncodersForType(t, fi); err != nil {
				fmt.Println("failed to generate encoders: ", err)
				os.Exit(1)
			}
		}
	}

	{
		fi, err := os.Create("./chain/cbor_gen.go")
		if err != nil {
			fmt.Println("failed to open file: ", err)
			os.Exit(1)
		}
		defer fi.Close()

		if err := gen.PrintHeaderAndUtilityMethods(fi, "chain"); err != nil {
			fmt.Println("failed to write header: ", err)
			os.Exit(1)
		}

		types := []interface{}{
			chain.BlockSyncRequest{},
			chain.BlockSyncResponse{},
			chain.BSTipSet{},
			chain.BlockMsg{},
		}

		for _, t := range types {
			if err := gen.GenTupleEncodersForType(t, fi); err != nil {
				fmt.Println("failed to generate encoders: ", err)
				os.Exit(1)
			}
		}
	}
}
