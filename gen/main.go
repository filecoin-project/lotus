package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/paychmgr"
)

func main() {
	err := gen.WriteTupleEncodersToFile("./chain/types/cbor_gen.go", "types",
		types.BlockHeader{},
		types.Ticket{},
		types.ElectionProof{},
		types.Message{},
		types.SignedMessage{},
		types.MsgMeta{},
		types.Actor{},
		types.MessageReceipt{},
		types.BlockMsg{},
		types.ExpTipSet{},
		types.BeaconEntry{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./paychmgr/cbor_gen.go", "paychmgr",
		paychmgr.VoucherInfo{},
		paychmgr.ChannelInfo{},
		paychmgr.MsgInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./api/cbor_gen.go", "api",
		api.PaymentInfo{},
		api.SealedRef{},
		api.SealedRefs{},
		api.SealTicket{},
		api.SealSeed{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./node/hello/cbor_gen.go", "hello",
		hello.HelloMessage{},
		hello.LatencyMessage{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./chain/blocksync/cbor_gen.go", "blocksync",
		blocksync.Request{},
		blocksync.Response{},
		blocksync.CompactedMessages{},
		blocksync.BSTipSet{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
