package deals

import (
	"context"
	"runtime"

	"github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborrpc"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
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
		State:    api.DealFailed,
		Message:  cerr.Error(),
		Proposal: id,
	})

	s, ok := h.conns[id]
	if ok {
		_ = s.Reset()
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

	worker, err := h.getWorker(h.actor)
	if err != nil {
		return err
	}

	sig, err := h.full.WalletSign(context.TODO(), worker, msg)
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

func (h *Handler) getWorker(miner address.Address) (address.Address, error) {
	getworker := &types.Message{
		To:     miner,
		From:   miner,
		Method: actors.MAMethods.GetWorkerAddr,
	}
	r, err := h.full.StateCall(context.TODO(), getworker, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting worker address: %w", err)
	}

	if r.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getWorker call failed: %d", r.ExitCode)
	}

	return address.NewFromBytes(r.Return)
}
