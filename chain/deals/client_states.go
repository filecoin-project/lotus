package deals

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
)

type clientHandlerFunc func(ctx context.Context, deal ClientDeal) error

func (c *Client) handle(ctx context.Context, deal ClientDeal, cb clientHandlerFunc, next api.DealState) {
	go func() {
		err := cb(ctx, deal)
		if err != nil {
			next = api.DealError
		}
		select {
		case c.updated <- clientDealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
		}:
		case <-c.stop:
		}
	}()
}

func (c *Client) new(ctx context.Context, deal ClientDeal) error {
	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != api.DealAccepted {
		return xerrors.Errorf("deal wasn't accepted (State=%d)", resp.State)
	}

	// TODO: spec says it's optional
	pubmsg, err := c.chain.GetMessage(*resp.PublishMessage)
	if err != nil {
		return xerrors.Errorf("getting deal pubsish message: %w", err)
	}

	pw, err := stmgr.GetMinerWorker(ctx, c.sm, nil, deal.Proposal.Provider)
	if err != nil {
		return xerrors.Errorf("getting miner worker failed: %w", err)
	}

	if pubmsg.From != pw {
		return xerrors.Errorf("deal wasn't published by storage provider: from=%s, provider=%s", pubmsg.From, deal.Proposal.Provider)
	}

	if pubmsg.To != actors.StorageMarketAddress {
		return xerrors.Errorf("deal publish message wasn't set to StorageMarket actor (to=%s)", pubmsg.To)
	}

	if pubmsg.Method != actors.SMAMethods.PublishStorageDeals {
		return xerrors.Errorf("deal publish message called incorrect method (method=%s)", pubmsg.Method)
	}

	// TODO: timeout
	_, ret, err := c.sm.WaitForMessage(ctx, *resp.PublishMessage)
	if err != nil {
		return xerrors.Errorf("waiting for deal publish message: %w", err)
	}
	if ret.ExitCode != 0 {
		return xerrors.Errorf("deal publish failed: exit=%d", ret.ExitCode)
	}
	// TODO: persist dealId

	log.Info("DEAL ACCEPTED!")

	return nil
}

func (c *Client) accepted(ctx context.Context, deal ClientDeal) error {
	/* data transfer happens */

	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != api.DealStaged {
		return xerrors.Errorf("deal wasn't staged (State=%d)", resp.State)
	}

	log.Info("DEAL STAGED!")

	return nil
}

func (c *Client) staged(ctx context.Context, deal ClientDeal) error {
	/* miner seals our data, hopefully */

	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != api.DealSealing {
		return xerrors.Errorf("deal wasn't sealed (State=%d)", resp.State)
	}

	log.Info("DEAL SEALED!")

	// TODO: want?
	/*ssize, err := stmgr.GetMinerSectorSize(ctx, c.sm, nil, deal.Proposal.MinerAddress)
	if err != nil {
		return xerrors.Errorf("failed to get miner sector size: %w", err)
	}

	ok, err := sectorbuilder.VerifyPieceInclusionProof(ssize, deal.Proposal.Size, deal.Proposal.CommP, resp.CommD, resp.PieceInclusionProof.ProofElements)
	if err != nil {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s: %w", deal.ProposalCid, err)
	}
	if !ok {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s failed", deal.ProposalCid)
	}*/

	return nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) error {
	//func (e *calledEvents) Called(check CheckFunc, hnd CalledHandler, rev RevertHandler, confidence int, timeout uint64, actor address.Address, method uint64) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		sd, err := stmgr.GetStorageDeal(ctx, c.stmgr, deal.DealID, ts)
		if err != nil {
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch > 0 {
			// Deal is active already!
			panic("handle me")
			return true, false, nil
		}

		return false, true, nil
	}

	called := func(msg *types.Message, ts *types.TipSet, curH uint64) (bool, error) {
		// To ask Magik: Does this trigger when the message in question is part of the parent state execution? Or just when its included in the block (aka, not executed)
		// main thing i want to ensure is that ts.ParentState is the result of the execution of msg

		if msg == nil {
			log.Error("timed out waiting for deal activation... what now?")
			return false, nil
		}

		// TODO: can check msg.Params to see if we should even bother checking the state

		sd, err := stmgr.GetStorageDeal(ctx, c.stmgr, deal.DealID, ts)
		if err != nil {
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch == 0 {
			return true, nil
		}

		// Deal is active!
		panic("handle me")

		return false, nil
	}

	if err := c.events.Called(checkFunc, handler, rev, 3, 100, actors.StorageMarketAddress, actors.SMAMethods.ActivateStorageDeals); err != nil {
		return xerrors.Errorf("failed to set up called handler")
	}
	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != api.DealComplete {
		return xerrors.Errorf("deal wasn't complete (State=%d)", resp.State)
	}

	// TODO: look for the commit message on chain, negotiate better payment vouchers

	log.Info("DEAL COMPLETE!!")
	return nil
}
