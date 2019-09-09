package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/types"
	gen "github.com/whyrusleeping/cbor-gen"
)

func main() {
	if false {
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

	if false {
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

	{
		fi, err := os.Create("./chain/actors/cbor_gen.go")
		if err != nil {
			fmt.Println("failed to open file: ", err)
			os.Exit(1)
		}
		defer fi.Close()

		if err := gen.PrintHeaderAndUtilityMethods(fi, "actors"); err != nil {
			fmt.Println("failed to write header: ", err)
			os.Exit(1)
		}

		types := []interface{}{
			actors.InitActorState{},
			actors.ExecParams{},
			actors.AccountActorState{},
			actors.StorageMinerActorState{},
			actors.StorageMinerConstructorParams{},
			actors.CommitSectorParams{},
			actors.MinerInfo{},
			actors.SubmitPoStParams{},
			actors.PieceInclVoucherData{},
			actors.InclusionProof{},
			actors.PaymentVerifyParams{},
			actors.UpdatePeerIDParams{},
			actors.MultiSigActorState{},
			actors.MultiSigConstructorParams{},
			actors.MultiSigProposeParams{},
			actors.MultiSigTxID{},
			actors.MultiSigSwapSignerParams{},
			actors.MultiSigChangeReqParams{},
			actors.MTransaction{},
			actors.MultiSigRemoveSignerParam{},
			actors.MultiSigAddSignerParam{},
			actors.PaymentChannelActorState{},
			actors.PCAConstructorParams{},
			actors.LaneState{},
			actors.PCAUpdateChannelStateParams{},
			actors.PaymentInfo{},
			actors.StorageMarketState{},
			actors.CreateStorageMinerParams{},
			actors.IsMinerParam{},
			actors.PowerLookupParams{},
			actors.UpdateStorageParams{},
		}

		for _, t := range types {
			if err := gen.GenTupleEncodersForType(t, fi); err != nil {
				fmt.Println("failed to generate encoders: ", err)
				os.Exit(1)
			}
		}
	}
}
