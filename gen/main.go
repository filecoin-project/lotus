package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	gen "github.com/whyrusleeping/cbor-gen"
)

func main() {
	err := gen.WriteTupleEncodersToFile("./chain/types/cbor_gen.go", "types",
		types.BlockHeader{},
		types.Ticket{},
		types.Message{},
		types.SignedMessage{},
		types.MsgMeta{},
		types.SignedVoucher{},
		types.ModVerifyParams{},
		types.Merge{},
		types.Actor{},
		types.MessageReceipt{},
		types.BlockMsg{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	/*
		err = gen.WriteTupleEncodersToFile("./chain/cbor_gen.go", "chain",
			chain.BlockSyncRequest{},
			chain.BlockSyncResponse{},
			chain.BSTipSet{},
		)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	*/

	err = gen.WriteTupleEncodersToFile("./chain/actors/cbor_gen.go", "actors",
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
		actors.StoragePowerState{},
		actors.CreateStorageMinerParams{},
		actors.IsMinerParam{},
		actors.PowerLookupParams{},
		actors.UpdateStorageParams{},
		actors.ArbitrateConsensusFaultParams{},
		actors.PledgeCollateralParams{},
		actors.MinerSlashConsensusFault{},
		actors.StorageParticipantBalance{},
		actors.StorageMarketState{},
		actors.WithdrawBalanceParams{},
		actors.StorageDealProposal{},
		actors.StorageDeal{},
		actors.PublishStorageDealsParams{},
		actors.PublishStorageDealResponse{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
