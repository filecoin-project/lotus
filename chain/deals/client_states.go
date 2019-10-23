package deals

import (
	"context"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"

	"golang.org/x/xerrors"
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

	if pubmsg.From != deal.Proposal.Provider {
		return xerrors.Errorf("Deal wasn't published by storage provider: from=%s, provider=%s", pubmsg.From, deal.Proposal.Provider)
	}

	if pubmsg.To != actors.StorageMarketAddress {
		return xerrors.Errorf("Deal publish message wasn't set to StorageMarket actor (to=%s)", pubmsg.To)
	}

	if pubmsg.Method != actors.SMAMethods.PublishStorageDeals {
		return xerrors.Errorf("Deal publish message called incorrect method (method=%s)", pubmsg.Method)
	}

	// TODO: timeout
	_, ret, err := c.sm.WaitForMessage(ctx, *resp.PublishMessage)
	if err != nil {
		return xerrors.Errorf("Waiting for deal publish message: %w", err)
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
	/*ok, err := sectorbuilder.VerifyPieceInclusionProof(build.SectorSize, deal.Proposal.PieceSize, deal.Proposal.CommP, resp.CommD, resp.PieceInclusionProof.ProofElements)
	if err != nil {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s: %w", deal.ProposalCid, err)
	}
	if !ok {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s failed", deal.ProposalCid)
	}*/

	return nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) error {
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
