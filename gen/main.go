package main

import (
	"fmt"
	"os"

	gen "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/exchange"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-shed/shedgen"
	"github.com/filecoin-project/lotus/conformance/chaos"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/paychmgr"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	sectorstorage "github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func main() {
	var lets errgroup.Group
	lets.Go(generateApiMaps)
	lets.Go(generateApiTuples)
	lets.Go(generateBlockstore)
	lets.Go(generateChainExchange)
	lets.Go(generateChainMarket)
	lets.Go(generateSnapshotMetadata)
	lets.Go(generateChainTypes)
	lets.Go(generateConformanceChaos)
	lets.Go(generateLotusShed)
	lets.Go(generateNodeHello)
	lets.Go(generatePaychmgr)
	lets.Go(generateStoragePipeline)
	lets.Go(generateStoragePipelinePiece)
	lets.Go(generateStorageSealer)
	lets.Go(generateStorageSealerInterface)
	if err := lets.Wait(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("All CBOR encoders have been generated successfully.")
}

func generateBlockstore() error {
	return gen.WriteTupleEncodersToFile("./blockstore/cbor_gen.go", "blockstore",
		blockstore.NetRpcReq{},
		blockstore.NetRpcResp{},
		blockstore.NetRpcErr{},
	)
}

func generateLotusShed() error {
	return gen.WriteMapEncodersToFile("./cmd/lotus-shed/shedgen/cbor_gen.go", "shedgen",
		shedgen.CarbNode{},
		shedgen.DatastoreEntry{},
	)
}

func generateStorageSealer() error {
	return gen.WriteMapEncodersToFile("./storage/sealer/cbor_gen.go", "sealer",
		sectorstorage.Call{},
		sectorstorage.WorkState{},
		sectorstorage.WorkID{},
	)
}

func generateStoragePipeline() error {
	return gen.WriteMapEncodersToFile("./storage/pipeline/cbor_gen.go", "sealing",
		sealing.SectorInfo{},
		sealing.Log{},
	)
}

func generateStoragePipelinePiece() error {
	return gen.WriteMapEncodersToFile("./storage/pipeline/piece/cbor_gen.go", "piece",
		piece.PieceDealInfo{},
		piece.DealSchedule{},
	)
}

func generateStorageSealerInterface() error {
	return gen.WriteMapEncodersToFile("./storage/sealer/storiface/cbor_gen.go", "storiface",
		storiface.CallID{},
		storiface.SecDataHttpHeader{},
		storiface.SectorLocation{},
	)
}

func generateChainExchange() error {
	return gen.WriteTupleEncodersToFile("./chain/exchange/cbor_gen.go", "exchange",
		exchange.Request{},
		exchange.Response{},
		exchange.CompactedMessagesCBOR{},
		exchange.BSTipSet{},
	)
}

func generateConformanceChaos() error {
	return gen.WriteTupleEncodersToFile("./conformance/chaos/cbor_gen.go", "chaos",
		chaos.State{},
		chaos.CallerValidationArgs{},
		chaos.CreateActorArgs{},
		chaos.ResolveAddressResponse{},
		chaos.SendArgs{},
		chaos.SendReturn{},
		chaos.MutateStateArgs{},
		chaos.AbortWithArgs{},
		chaos.InspectRuntimeReturn{},
	)
}

func generateChainMarket() error {
	return gen.WriteTupleEncodersToFile("./chain/market/cbor_gen.go", "market",
		market.FundedAddressState{},
	)
}

func generateNodeHello() error {
	return gen.WriteTupleEncodersToFile("./node/hello/cbor_gen.go", "hello",
		hello.HelloMessage{},
		hello.LatencyMessage{},
	)
}

func generateApiMaps() error {
	return gen.WriteMapEncodersToFile("./api/cbor_gen.go", "api",
		api.PaymentInfo{},
		api.SealedRef{},
		api.SealedRefs{},
		api.SealTicket{},
		api.SealSeed{},
		api.SectorPiece{},
	)
}

func generateApiTuples() error {
	return gen.WriteTupleEncodersToFile("./api/cbor_tuples_gen.go", "api",
		api.F3ParticipationLease{},
	)
}

func generatePaychmgr() error {
	return gen.WriteMapEncodersToFile("./paychmgr/cbor_gen.go", "paychmgr",
		paychmgr.VoucherInfo{},
		paychmgr.ChannelInfo{},
		paychmgr.MsgInfo{},
	)
}

func generateSnapshotMetadata() error {
	return gen.WriteMapEncodersToFile("./chain/store/cbor_gen.go", "store",
		store.SnapshotMetadata{},
	)
}

func generateChainTypes() error {
	return gen.WriteTupleEncodersToFile("./chain/types/cbor_gen.go", "types",
		types.BlockHeader{},
		types.Ticket{},
		types.ElectionProof{},
		types.Message{},
		types.SignedMessage{},
		types.MsgMeta{},
		types.ActorV4{},
		types.ActorV5{},
		// types.MessageReceipt{}, // Custom serde to deal with versioning.
		types.BlockMsg{},
		types.ExpTipSet{},
		types.BeaconEntry{},
		types.StateRoot{},
		types.StateInfo0{},
		types.Event{},
		types.EventEntry{},
		// Tracing
		types.GasTrace{},
		types.ActorTrace{},
		types.MessageTrace{},
		types.ReturnTrace{},
		types.TraceIpld{},
		types.ExecutionTrace{},
	)
}
