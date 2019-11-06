package deals

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

const DealProtocolID = "/fil/storage/mk/1.0.1"
const AskProtocolID = "/fil/storage/ask/1.0.1"

type Proposal struct {
	DealProposal *actors.StorageDealProposal

	Piece cid.Cid // Used for retrieving from the client
}

type Response struct {
	State api.DealState

	// DealProposalRejected
	Message  string
	Proposal cid.Cid

	// DealAccepted
	StorageDeal    *actors.StorageDeal
	PublishMessage *cid.Cid

	// DealComplete
	CommitMessage *cid.Cid
}

// TODO: Do we actually need this to be signed?
type SignedResponse struct {
	Response Response

	Signature *types.Signature
}

type AskRequest struct {
	Miner address.Address
}

type AskResponse struct {
	Ask *types.SignedStorageAsk
}
