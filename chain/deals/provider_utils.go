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
	inet "github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"
)

func (p *Provider) failDeal(id cid.Cid, cerr error) {
	if err := p.deals.End(id); err != nil {
		log.Warnf("deals.End: %s", err)
	}

	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	log.Warnf("deal %s failed: %s", id, cerr)

	err := p.sendSignedResponse(&Response{
		State:    api.DealFailed,
		Message:  cerr.Error(),
		Proposal: id,
	})

	s, ok := p.conns[id]
	if ok {
		_ = s.Reset()
		delete(p.conns, id)
	}

	if err != nil {
		log.Warnf("notifying client about deal failure: %s", err)
	}
}

func (p *Provider) readProposal(s inet.Stream) (proposal Proposal, err error) {
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return proposal, err
	}

	if err := proposal.DealProposal.Verify(); err != nil {
		return proposal, xerrors.Errorf("verifying StorageDealProposal: %w", err)
	}

	if proposal.DealProposal.Provider != p.actor {
		log.Errorf("proposal with wrong ProviderAddress: %s", proposal.DealProposal.Provider)
		return proposal, err
	}

	return
}

func (p *Provider) sendSignedResponse(resp *Response) error {
	s, ok := p.conns[resp.Proposal]
	if !ok {
		return xerrors.New("couldn't send response: not connected")
	}

	msg, err := cborrpc.Dump(resp)
	if err != nil {
		return xerrors.Errorf("serializing response: %w", err)
	}

	worker, err := p.getWorker(p.actor)
	if err != nil {
		return err
	}

	sig, err := p.full.WalletSign(context.TODO(), worker, msg)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := &SignedResponse{
		Response:  *resp,
		Signature: sig,
	}

	err = cborrpc.WriteCborRPC(s, signedResponse)
	if err != nil {
		// Assume client disconnected
		s.Close()
		delete(p.conns, resp.Proposal)
	}
	return err
}

func (p *Provider) disconnect(deal MinerDeal) error {
	s, ok := p.conns[deal.ProposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(p.conns, deal.ProposalCid)
	return err
}

func (p *Provider) getWorker(miner address.Address) (address.Address, error) {
	getworker := &types.Message{
		To:     miner,
		From:   miner,
		Method: actors.MAMethods.GetWorkerAddr,
	}
	r, err := p.full.StateCall(context.TODO(), getworker, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting worker address: %w", err)
	}

	if r.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getWorker call failed: %d", r.ExitCode)
	}

	return address.NewFromBytes(r.Return)
}
