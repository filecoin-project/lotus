package deals

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func init() {
	cbor.RegisterCborType(StorageDealProposal{})
	cbor.RegisterCborType(SignedStorageDealProposal{})

	cbor.RegisterCborType(PieceInclusionProof{})

	cbor.RegisterCborType(StorageDealResponse{})
	cbor.RegisterCborType(SignedStorageDealResponse{})

	cbor.RegisterCborType(AskRequest{})
	cbor.RegisterCborType(AskResponse{})
}

const ProtocolID = "/fil/storage/mk/1.0.0"
const AskProtocolID = "/fil/storage/ask/1.0.0"

type SerializationMode string

const (
	SerializationUnixFs = "UnixFs"
	SerializationRaw    = "Raw"
	SerializationIPLD   = "IPLD"
)

type StorageDealProposal struct {
	PieceRef          cid.Cid // TODO: port to spec
	SerializationMode SerializationMode
	CommP             []byte

	Size       uint64
	TotalPrice types.BigInt
	Duration   uint64

	Payment actors.PaymentInfo

	MinerAddress  address.Address
	ClientAddress address.Address
}

type SignedStorageDealProposal struct {
	Proposal StorageDealProposal

	Signature *types.Signature
}

// response

type PieceInclusionProof struct {
	Position      uint64
	ProofElements []byte
}

type StorageDealResponse struct {
	State api.DealState

	// DealRejected / DealAccepted / DealFailed / DealStaged
	Message  string
	Proposal cid.Cid

	// DealSealing
	PieceInclusionProof PieceInclusionProof
	CommD               []byte // TODO: not in spec

	// DealComplete
	SectorCommitMessage *cid.Cid
}

type SignedStorageDealResponse struct {
	Response StorageDealResponse

	Signature *types.Signature
}

type AskRequest struct {
	Miner address.Address
}

type AskResponse struct {
	Ask *types.SignedStorageAsk
}
