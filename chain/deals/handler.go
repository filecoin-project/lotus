package deals

import (
	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	inet "github.com/libp2p/go-libp2p-core/network"
)

type Handler struct {
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) HandleStream(s inet.Stream) {
	defer s.Close()

	var proposal SignedStorageDealProposal
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return
	}

	// TODO: Validate proposal maybe
	// (and signature, obviously)

	response := StorageDealResponse{
		State:    Accepted,
		Message:  "",
		Proposal: nil, // TODO
	}
	signedResponse := &SignedStorageDealResponse{
		Response:  response,
		Signature: nil, // TODO
	}
	if err := cborrpc.WriteCborRPC(s, signedResponse); err != nil {
		log.Errorw("failed to write deal response", "error", err)
		return
	}

}
