package deals

import (
	"context"
	"github.com/filecoin-project/lotus/build"

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

		if err == nil && next == api.DealNoUpdate {
			return
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
	if err := c.disconnect(deal); err != nil {
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
	// wait

	return nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		sd, err := stmgr.GetStorageDeal(ctx, c.sm, deal.DealID, ts)
		if err != nil {
			// TODO: This may be fine for some errors
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch > 0 {
			select {
			case c.updated <- clientDealUpdate{
				newState: api.DealComplete,
				id:       deal.ProposalCid,
			}:
			case <-c.stop:
			}

			return true, false, nil
		}

		return false, true, nil
	}

	called := func(msg *types.Message, ts *types.TipSet, curH uint64) (bool, error) {
		// TODO: handle errors

		if msg == nil {
			log.Error("timed out waiting for deal activation... what now?")
			return false, nil
		}

		// TODO: can check msg.Params to see if we should even bother checking the state

		sd, err := stmgr.GetStorageDeal(ctx, c.sm, deal.DealID, ts)
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch == 0 {
			return true, nil
		}

		log.Info("Storage deal %d activated at epoch %d", deal.DealID, sd.ActivationEpoch)

		select {
		case c.updated <- clientDealUpdate{
			newState: api.DealComplete,
			id:       deal.ProposalCid,
		}:
		case <-c.stop:
		}

		return false, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	if err := c.events.Called(checkFunc, called, revert, 3, build.SealRandomnessLookbackLimit, deal.Proposal.Provider, actors.MAMethods.ProveCommitSector); err != nil {
		return xerrors.Errorf("failed to set up called handler")
	}

	log.Info("DEAL COMPLETE!!")
	return nil
}
