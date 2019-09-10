package deals

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
)

type clientHandlerFunc func(ctx context.Context, deal ClientDeal) error

func (c *Client) handle(ctx context.Context, deal ClientDeal, cb clientHandlerFunc, next DealState) {
	go func() {
		err := cb(ctx, deal)
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

	if resp.State != Accepted {
		return xerrors.Errorf("deal wasn't accepted (State=%d)", resp.State)
	}

	log.Info("DEAL ACCEPTED!")

	return nil
}

func (c *Client) accepted(ctx context.Context, deal ClientDeal) error {
	/* data transfer happens */

	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != Staged {
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

	if resp.State != Sealing {
		return xerrors.Errorf("deal wasn't sealed (State=%d)", resp.State)
	}

	log.Info("DEAL SEALED!")

	ok, err := sectorbuilder.VerifyPieceInclusionProof(build.SectorSize, deal.Proposal.Size, deal.Proposal.CommP, resp.CommD, resp.PieceInclusionProof.ProofElements)
	if err != nil {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s: %w", deal.ProposalCid, err)
	}
	if !ok {
		return xerrors.Errorf("verifying piece inclusion proof in staged deal %s failed", deal.ProposalCid)
	}

	return nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) error {
	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return err
	}

	if resp.State != Complete {
		return xerrors.Errorf("deal wasn't complete (State=%d)", resp.State)
	}

	// TODO: look for the commit message on chain, negotiate better payment vouchers

	log.Info("DEAL COMPLETE!!")
	return nil
}
