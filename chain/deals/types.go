package deals

import (
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {
	cbor.RegisterCborType(StorageDealResponse{})
	cbor.RegisterCborType(SignedStorageDealResponse{})

	cbor.RegisterCborType(AskRequest{})
	cbor.RegisterCborType(AskResponse{})
}

const DealProtocolID = "/fil/storage/mk/1.0.0"
const AskProtocolID = "/fil/storage/ask/1.0.0"

type Proposal struct {
	DealProposal actors.StorageDealProposal
}

type StorageDealResponse struct {
	State api.DealState

	// DealProposalRejected
	Message  string
	Proposal cid.Cid

	// DealAccepted
	StorageDeal actors.StorageDeal
	PublishMessage cid.Cid

	// DealComplete
	CommitMessage cid.Cid
}

// TODO: Do we actually need this to be signed?
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
