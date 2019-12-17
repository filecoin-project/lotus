package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/paych"
	"github.com/filecoin-project/lotus/storage"
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

	err = gen.WriteTupleEncodersToFile("./chain/actors/cbor_gen.go", "actors",
		actors.InitActorState{},
		actors.ExecParams{},
		actors.AccountActorState{},
		actors.StorageMinerActorState{},
		actors.StorageMinerConstructorParams{},
		actors.SectorPreCommitInfo{},
		actors.PreCommittedSector{},
		actors.MinerInfo{},
		actors.SubmitFallbackPoStParams{},
		actors.PaymentVerifyParams{},
		actors.UpdatePeerIDParams{},
		actors.DeclareFaultsParams{},
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
		actors.IsValidMinerParam{},
		actors.PowerLookupParams{},
		actors.UpdateStorageParams{},
		actors.ArbitrateConsensusFaultParams{},
		actors.PledgeCollateralParams{},
		actors.MinerSlashConsensusFault{},
		actors.StorageParticipantBalance{},
		actors.StorageMarketState{},
		actors.WithdrawBalanceParams{},
		actors.StorageDealProposal{},
		actors.PublishStorageDealsParams{},
		actors.PublishStorageDealResponse{},
		actors.ActivateStorageDealsParams{},
		actors.ProcessStorageDealsPaymentParams{},
		actors.OnChainDeal{},
		actors.ComputeDataCommitmentParams{},
		actors.SectorProveCommitInfo{},
		actors.CheckMinerParams{},
		actors.CronActorState{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = gen.WriteMapEncodersToFile("./storage/cbor_gen.go", "storage",
		storage.SealTicket{},
		storage.SealSeed{},
		storage.Piece{},
		storage.SectorInfo{},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
