package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/deals"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/paych"
	"github.com/filecoin-project/lotus/retrieval"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/datatransfer/message"
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
		types.SignedStorageAsk{},
		types.StorageAsk{},
		types.ExpTipSet{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./paych/cbor_gen.go", "paych",
		paych.VoucherInfo{},
		paych.ChannelInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./api/cbor_gen.go", "api",
		api.PaymentInfo{},
		api.SealedRef{},
		api.SealedRefs{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./retrieval/cbor_gen.go", "retrieval",
		retrieval.RetParams{},

		retrieval.Query{},
		retrieval.QueryResponse{},
		retrieval.Unixfs0Offer{},
		retrieval.DealProposal{},
		retrieval.DealResponse{},
		retrieval.Block{},
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

	err = gen.WriteTupleEncodersToFile("./chain/actors/cbor_gen.go", "actors",
		actors.InitActorState{},
		actors.ExecParams{},
		actors.AccountActorState{},
		actors.StorageMinerActorState{},
		actors.StorageMinerConstructorParams{},
		actors.SectorPreCommitInfo{},
		actors.PreCommittedSector{},
		actors.MinerInfo{},
		actors.SubmitPoStParams{},
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
		actors.ActivateStorageDealsParams{},
		actors.ProcessStorageDealsPaymentParams{},
		actors.OnChainDeal{},
		actors.ComputeDataCommitmentParams{},
		actors.SectorProveCommitInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./chain/deals/cbor_gen.go", "deals",
		deals.AskRequest{},
		deals.AskResponse{},
		deals.Proposal{},
		deals.Response{},
		deals.SignedResponse{},
		deals.ClientDealProposal{},
		deals.ClientDeal{},
		deals.MinerDeal{},
		deals.StorageDataTransferVoucher{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./storage/cbor_gen.go", "storage",
		storage.SealTicket{},
		storage.SealSeed{},
		storage.Piece{},
		storage.SectorInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteTupleEncodersToFile("./datatransfer/message/cbor_gen.go", "message",
		message.TransferRequest{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
