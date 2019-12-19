package deals

import (
	"bytes"
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/go-cbor-util"
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
		info.PublishMessage = resp.StorageDealSubmission
	}, nil
}

func (c *Client) accepted(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	log.Infow("DEAL ACCEPTED!")

	pubmsg := deal.PublishMessage.Message
	pw, err := stmgr.GetMinerWorker(ctx, c.sm, nil, deal.Proposal.Provider)
	if err != nil {
		return nil, xerrors.Errorf("getting miner worker failed: %w", err)
	}

	if pubmsg.From != pw {
		return nil, xerrors.Errorf("deal wasn't published by storage provider: from=%s, provider=%s", pubmsg.From, deal.Proposal.Provider)
	}

	if pubmsg.To != actors.StorageMarketAddress {
		return nil, xerrors.Errorf("deal publish message wasn't set to StorageMarket actor (to=%s)", pubmsg.To)
	}

	if pubmsg.Method != actors.SMAMethods.PublishStorageDeals {
		return nil, xerrors.Errorf("deal publish message called incorrect method (method=%s)", pubmsg.Method)
	}

	var params actors.PublishStorageDealsParams
	if err := params.UnmarshalCBOR(bytes.NewReader(pubmsg.Params)); err != nil {
		return nil, err
	}

	dealIdx := -1
	for i, storageDeal := range params.Deals {
		// TODO: make it less hacky
		sd := storageDeal
		eq, err := cborutil.Equals(&deal.Proposal, &sd)
		if err != nil {
			return nil, err
		}
		if eq {
			dealIdx = i
			break
		}
	}

	if dealIdx == -1 {
		return nil, xerrors.Errorf("deal publish didn't contain our deal (message cid: %s)", deal.PublishMessage.Cid())
	}

	// TODO: timeout
	_, ret, err := c.sm.WaitForMessage(ctx, deal.PublishMessage.Cid())
	if err != nil {
		return nil, xerrors.Errorf("waiting for deal publish message: %w", err)
	}
	if ret.ExitCode != 0 {
		return nil, xerrors.Errorf("deal publish failed: exit=%d", ret.ExitCode)
	}

	var res actors.PublishStorageDealResponse
	if err := res.UnmarshalCBOR(bytes.NewReader(ret.Return)); err != nil {
		return nil, err
	}

	return func(info *ClientDeal) {
		info.DealID = res.DealIDs[dealIdx]
	}, nil
}

func (c *Client) staged(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
	// TODO: Maybe wait for pre-commit

	return nil, nil
}

func (c *Client) sealing(ctx context.Context, deal ClientDeal) (func(*ClientDeal), error) {
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

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH uint64) (more bool, err error) {
		defer func() {
			if err != nil {
				select {
				case c.updated <- clientDealUpdate{
					newState: api.DealComplete,
					id:       deal.ProposalCid,
					err:      xerrors.Errorf("handling applied event: %w", err),
				}:
				case <-c.stop:
				}
			}
		}()

		if msg == nil {
			log.Error("timed out waiting for deal activation... what now?")
			return false, nil
		}

		sd, err := stmgr.GetStorageDeal(ctx, c.sm, deal.DealID, ts)
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch == 0 {
			return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", deal.DealID, ts.ParentState(), ts.Height())
		}

		log.Infof("Storage deal %d activated at epoch %d", deal.DealID, sd.ActivationEpoch)

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

	matchEvent := func(msg *types.Message) (bool, error) {
		if msg.To != deal.Proposal.Provider {
			return false, nil
		}

		if msg.Method != actors.MAMethods.ProveCommitSector {
			return false, nil
		}

		var params actors.SectorProveCommitInfo
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, err
		}

		var found bool
		for _, dealID := range params.DealIDs {
			if dealID == deal.DealID {
				found = true
				break
			}
		}

		return found, nil
	}

	if err := c.events.Called(checkFunc, called, revert, 3, build.SealRandomnessLookbackLimit, matchEvent); err != nil {
		return nil, xerrors.Errorf("failed to set up called handler")
	}

	return nil, nil
}
