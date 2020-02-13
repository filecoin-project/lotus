package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/statemachine"
	"github.com/filecoin-project/lotus/paychmgr"
	"github.com/filecoin-project/lotus/storage/sealing"
)

func main() {
	err := gen.WriteTupleEncodersToFile("./chain/types/cbor_gen.go", "types",
		types.BlockHeader{},
		types.Ticket{},
		types.EPostProof{},
		types.EPostTicket{},
		types.Message{},
		types.SignedMessage{},
		types.MsgMeta{},
		types.Actor{},
		types.MessageReceipt{},
		types.BlockMsg{},
		types.ExpTipSet{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./paychmgr/cbor_gen.go", "paychmgr",
		paychmgr.VoucherInfo{},
		paychmgr.ChannelInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./api/cbor_gen.go", "api",
		api.PaymentInfo{},
		api.SealedRef{},
		api.SealedRefs{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./chain/blocksync/cbor_gen.go", "blocksync",
		blocksync.BlockSyncRequest{},
		blocksync.BlockSyncResponse{},
		blocksync.BSTipSet{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./storage/sealing/cbor_gen.go", "sealing",
		sealing.SealTicket{},
		sealing.SealSeed{},
		sealing.Piece{},
		sealing.SectorInfo{},
		sealing.Log{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./lib/statemachine/cbor_gen.go", "statemachine",
		statemachine.TestState{},
		statemachine.TestEvent{},
	)
	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}
