package storageadapter

import (
	"context"

	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
)

type demEventsAPI interface {
	Called(check events.CheckFunc, msgHnd events.MsgHandler, rev events.RevertHandler, confidence int, timeout abi.ChainEpoch, mf events.MsgMatchFunc) error
	StateChanged(check events.CheckFunc, scHnd events.StateChangeHandler, rev events.RevertHandler, confidence int, timeout abi.ChainEpoch, mf events.StateMatchFunc) error
}

type demChainAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
}

type DealExpiryManagerAPI interface {
	demEventsAPI
	demChainAPI
	GetCurrentDealInfo(ctx context.Context, tok sealing.TipSetToken, proposal *market.DealProposal, publishCid cid.Cid) (sealing.CurrentDealInfo, error)
}

type dealExpiryManagerAdapter struct {
	demEventsAPI
	demChainAPI
	*sealing.CurrentDealInfoManager
}

type DealExpiryManager struct {
	demAPI    DealExpiryManagerAPI
	dsMatcher *dealStateMatcher
}

func NewDealExpiryManager(ev demEventsAPI, ch demChainAPI, tskAPI sealing.CurrentDealInfoTskAPI, dsMatcher *dealStateMatcher) *DealExpiryManager {
	dim := &sealing.CurrentDealInfoManager{
		CDAPI: &sealing.CurrentDealInfoAPIAdapter{CurrentDealInfoTskAPI: tskAPI},
	}

	adapter := &dealExpiryManagerAdapter{
		demEventsAPI:           ev,
		demChainAPI:            ch,
		CurrentDealInfoManager: dim,
	}
	return newDealExpiryManager(adapter, dsMatcher)
}

func newDealExpiryManager(demAPI DealExpiryManagerAPI, dsMatcher *dealStateMatcher) *DealExpiryManager {
	return &DealExpiryManager{demAPI: demAPI, dsMatcher: dsMatcher}
}

func (mgr *DealExpiryManager) OnDealExpiredOrSlashed(ctx context.Context, publishCid cid.Cid, proposal market0.DealProposal, onDealExpired storagemarket.DealExpiredCallback, onDealSlashed storagemarket.DealSlashedCallback) error {
	head, err := mgr.demAPI.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("client: failed to get chain head: %w", err)
	}

	prop := market.DealProposal(proposal)
	res, err := mgr.demAPI.GetCurrentDealInfo(ctx, head.Key().Bytes(), &prop, publishCid)
	if err != nil {
		return xerrors.Errorf("awaiting deal expired: getting deal info: %w", err)
	}

	// Called immediately to check if the deal has already expired or been slashed
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		if ts == nil {
			// keep listening for events
			return false, true, nil
		}

		// Check if the deal has already expired
		if res.MarketDeal.Proposal.EndEpoch <= ts.Height() {
			onDealExpired(nil)
			return true, false, nil
		}

		// If there is no deal assume it's already been slashed
		if res.MarketDeal.State.SectorStartEpoch < 0 {
			onDealSlashed(ts.Height(), nil)
			return true, false, nil
		}

		// No events have occurred yet, so return
		// done: false, more: true (keep listening for events)
		return false, true, nil
	}

	// Called when there was a match against the state change we're looking for
	// and the chain has advanced to the confidence height
	stateChanged := func(ts *types.TipSet, ts2 *types.TipSet, states events.StateChange, h abi.ChainEpoch) (more bool, err error) {
		// Check if the deal has already expired
		if res.MarketDeal.Proposal.EndEpoch <= h {
			onDealExpired(nil)
			return false, nil
		}

		// Timeout waiting for state change
		if states == nil {
			log.Error("timed out waiting for deal expiry")
			return false, nil
		}

		changedDeals, ok := states.(state.ChangedDeals)
		if !ok {
			return false, xerrors.Errorf("OnDealExpireOrSlashed stateChanged: Expected state.ChangedDeals")
		}

		deal, ok := changedDeals[res.DealID]
		if !ok {
			// No change to deal
			return true, nil
		}

		// Deal was slashed
		if deal.To == nil {
			onDealSlashed(ts2.Height(), nil)
			return false, nil
		}

		return true, nil
	}

	// Called when there was a chain reorg and the state change was reverted
	revert := func(ctx context.Context, ts *types.TipSet) error {
		// TODO: Is it ok to just ignore this?
		log.Warn("deal state reverted; TODO: actually handle this!")
		return nil
	}

	// Watch for state changes to the deal
	match := mgr.dsMatcher.matcher(ctx, res.DealID)

	// Wait until after the end epoch for the deal and then timeout
	timeout := res.MarketDeal.Proposal.EndEpoch + 1
	if err := mgr.demAPI.StateChanged(checkFunc, stateChanged, revert, int(build.MessageConfidence)+1, timeout, match); err != nil {
		return xerrors.Errorf("failed to set up state changed handler: %w", err)
	}

	return nil
}
