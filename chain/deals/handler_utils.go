package deals

import (
	"runtime"

	"github.com/filecoin-project/go-lotus/lib/cborrpc"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

func (h *Handler) failDeal(id cid.Cid, cerr error) {
	if err := h.deals.End(id); err != nil {
		log.Warnf("deals.End: %s", err)
	}

	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	log.Errorf("deal %s failed: %s", id, cerr)

	err := h.sendSignedResponse(StorageDealResponse{
		State:    Failed,
		Message:  cerr.Error(),
		Proposal: id,
	})

	s, ok := h.conns[id]
	if ok {
		_ = s.Close()
		delete(h.conns, id)
	}

	if err != nil {
		log.Warnf("notifying client about deal failure: %s", err)
	}
}

func (h *Handler) readProposal(s inet.Stream) (proposal SignedStorageDealProposal, err error) {
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return SignedStorageDealProposal{}, err
	}

	// TODO: Validate proposal maybe
	// (and signature, obviously)

	if proposal.Proposal.MinerAddress != h.actor {
		log.Errorf("proposal with wrong MinerAddress: %s", proposal.Proposal.MinerAddress)
		return SignedStorageDealProposal{}, err
	}

	return
}

func (h *Handler) sendSignedResponse(resp StorageDealResponse) error {
	s, ok := h.conns[resp.Proposal]
	if !ok {
		return xerrors.New("couldn't send response: not connected")
	}

	msg, err := cbor.DumpObject(&resp)
	if err != nil {
		return xerrors.Errorf("serializing response: %w", err)
	}

	def, err := h.w.ListAddrs()
	if err != nil {
		log.Error(err)
		return xerrors.Errorf("listing wallet addresses: %w", err)
	}
	if len(def) != 1 {
		// NOTE: If this ever happens for a good reason, implement this with GetWorker on the miner actor
		// TODO: implement with GetWorker on the miner actor
		return xerrors.Errorf("expected only 1 address in wallet, got %d", len(def))
	}

	sig, err := h.w.Sign(def[0], msg)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := SignedStorageDealResponse{
		Response:  resp,
		Signature: sig,
	}

	err = cborrpc.WriteCborRPC(s, signedResponse)
	if err != nil {
		// Assume client disconnected
		s.Close()
		delete(h.conns, resp.Proposal)
	}
	return err
}
