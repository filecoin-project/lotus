package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/exchange"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/cmd/lotus-shed/shedgen"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/paychmgr"
	sectorstorage "github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
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
		types.StateRoot{},
		types.StateInfo0{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./chain/vm/cbor_gen.go", "vm",
		vm.FvmExecutionTrace{},
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
		api.PieceDealInfo{},
		api.DealSchedule{},
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

	err = gen.WriteTupleEncodersToFile("./chain/market/cbor_gen.go", "market",
		market.FundedAddressState{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./chain/exchange/cbor_gen.go", "exchange",
		exchange.Request{},
		exchange.Response{},
		exchange.CompactedMessages{},
		exchange.BSTipSet{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./storage/sealer/storiface/cbor_gen.go", "storiface",
		storiface.CallID{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./storage/sealer/cbor_gen.go", "sealer",
		sectorstorage.Call{},
		sectorstorage.WorkState{},
		sectorstorage.WorkID{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = gen.WriteMapEncodersToFile("./cmd/lotus-shed/shedgen/cbor_gen.go", "shedgen",
		shedgen.CarbNode{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
