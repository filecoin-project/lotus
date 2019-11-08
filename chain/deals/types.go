package deals

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
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
}

// TODO: Do we actually need this to be signed?
type SignedResponse struct {
	Response Response

	Signature *types.Signature
}

func (r *SignedResponse) Verify(addr address.Address) error {
	b, err := cborutil.Dump(&r.Response)
	if err != nil {
		return err
	}

	return r.Signature.Verify(addr, b)
}

type AskRequest struct {
	Miner address.Address
}

type AskResponse struct {
	Ask *types.SignedStorageAsk
}
