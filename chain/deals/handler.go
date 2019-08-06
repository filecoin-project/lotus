package deals

import (
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	"math"

	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
)

type Handler struct {
	w *wallet.Wallet
}

func NewHandler(w *wallet.Wallet) *Handler {
	return &Handler{
		w: w,
	}
}

func (h *Handler) HandleStream(s inet.Stream) {
	defer s.Close()

	log.Info("Handling storage deal proposal!")

	var proposal SignedStorageDealProposal
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return
	}

	// TODO: Validate proposal maybe
	// (and signature, obviously)

	// TODO: Review: Not signed?
	proposalNd, err := cbor.WrapObject(proposal.Proposal, math.MaxUint64, -1)
	if err != nil {
		return
	}

	response := StorageDealResponse{
		State:   Accepted,
		Message: "",
		Proposal: proposalNd.Cid(),
	}

	msg, err := cbor.DumpObject(response)
	if err != nil {
		log.Errorw("failed to serialize response message", "error", err)
		return
	}
	sig, err := h.w.Sign(proposal.Proposal.MinerAddress, msg)
	if err != nil {
		log.Errorw("failed to sign response message", "error", err)
		return
	}

	signedResponse := &SignedStorageDealResponse{
		Response: response,
		Signature: sig,
	}
	if err := cborrpc.WriteCborRPC(s, signedResponse); err != nil {
		log.Errorw("failed to write deal response", "error", err)
		return
	}

}
