package deals

import (
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
}

const ProtocolID = "/fil/storage/mk/1.0.0"

type SerializationMode string

const (
	SerializationUnixFs = "UnixFs"
	SerializationRaw    = "Raw"
	SerializationIPLD   = "IPLD"
)

type DealState int

const (
	Unknown = iota
	Rejected
	Accepted
	Started
	Failed
	Staged
	Sealing
	Complete
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
	State DealState

	// Rejected / Accepted / Failed / Staged
	Message  string
	Proposal cid.Cid

	// Sealing
	PieceInclusionProof PieceInclusionProof
	CommD               []byte // TODO: not in spec

	// Complete
	SectorCommitMessage *cid.Cid
}

type SignedStorageDealResponse struct {
	Response StorageDealResponse

	Signature *types.Signature
}
