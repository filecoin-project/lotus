 package deals

 import (
	 "github.com/filecoin-project/go-lotus/api"
	 "github.com/ipfs/go-cid"
	 cbor "github.com/ipfs/go-ipld-cbor"

	 "github.com/filecoin-project/go-lotus/chain/address"
	 "github.com/filecoin-project/go-lotus/chain/types"
 )

func init() {
	cbor.RegisterCborType(UnsignedStorageDealProposal{})
	cbor.RegisterCborType(StorageDealProposal{})

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

type UnsignedStorageDealProposal struct {
	PieceRef          cid.Cid // TODO: port to spec
	PieceSize       uint64

	Client   address.Address
	Provider address.Address

	ProposalExpiryEpoch uint64
	DealExpiryEpoch uint64

	StoragePrice         types.BigInt
	StorageCollateral    types.BigInt

	ProposerSignature    *types.Signature
}

type StorageDealProposal struct {
	UnsignedStorageDealProposal // TODO: check bytes

	ProposerSignature *types.Signature
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
