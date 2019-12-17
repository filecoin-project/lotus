package deals

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type clientHandlerFunc func(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error)

func (c *Client) handle(ctx context.Context, deal ClientDeal, cb clientHandlerFunc, next api.DealState) {
	go func() {
		mut, err := cb(ctx, deal)
		if err != nil {
			next = api.DealError
		}

		if err == nil && next == api.DealNoUpdate {
			return
		}

		select {
		case c.updated <- clientDealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
			mut:      mut,
		}:
		case <-c.stop:
		}
	}()
}

func (c *Client) new(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	resp, err := c.readStorageDealResp(deal)
	if err != nil {
		return nil, err
	}

	// TODO: verify StorageDealSubmission

	if err := c.disconnect(deal); err != nil {
		return nil, err
	}

	/* data transfer happens */
	if resp.State != api.DealAccepted {
		return nil, xerrors.Errorf("deal wasn't accepted (State=%d)", resp.State)
	}

	return func(info *ClientDeal) {
		info.PublishMessage = resp.PublishMessage
	}, nil
}

func (c *Client) accepted(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	log.Infow("DEAL ACCEPTED!")

	dealId, err := c.node.ValidatePublishedDeal(ctx, deal.ClientDeal)
	if err != nil {
		return nil, err
	}

	return func(info *ClientDeal) {
		info.DealID = dealId
	}, nil
}

func (c *Client) staged(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	// TODO: Maybe wait for pre-commit

	return nil, nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	cb := func(err error) {
		select {
		case c.updated <- clientDealUpdate{
			newState: api.DealComplete,
			id:       deal.ProposalCid,
			err:      err,
		}:
		case <-c.stop:
		}
	}

	err := c.node.OnDealSectorCommitted(ctx, deal.Proposal.Provider, deal.DealID, cb)

	return nil, err
}

func (c *Client) checkAskSignature(ask *types.SignedStorageAsk) error {
	return c.node.ValidateAskSignature(ask)
}
