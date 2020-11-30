package storageadapter

import (
	"bytes"
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type sectorCommittedEventsAPI interface {
	Called(check events.CheckFunc, msgHnd events.MsgHandler, rev events.RevertHandler, confidence int, timeout abi.ChainEpoch, mf events.MsgMatchFunc) error
	ChainAt(hnd events.HeightHandler, rev events.RevertHandler, confidence int, h abi.ChainEpoch) error
}

func OnDealSectorPreCommitted(ctx context.Context, api getCurrentDealInfoAPI, eventsApi sectorCommittedEventsAPI, provider address.Address, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid, callback storagemarket.DealSectorPreCommittedCallback) error {
	// Ensure callback is only called once
	var once sync.Once
	cb := func(sectorNumber abi.SectorNumber, isActive bool, err error) {
		once.Do(func() {
			callback(sectorNumber, isActive, err)
		})
	}

	// First check if the deal is already active, and if so, bail out
	var proposedDealStartEpoch abi.ChainEpoch
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		deal, isActive, err := checkIfDealAlreadyActive(ctx, api, ts, dealID, proposal, publishCid)
		if err != nil {
			// Note: the error returned from here will end up being returned
			// from OnDealSectorPreCommitted so no need to call the callback
			// with the error
			return false, false, err
		}

		if isActive {
			// Deal is already active, bail out
			cb(0, true, nil)
			return true, false, nil
		}

		// Save the proposed deal start epoch so we can timeout if the deal
		// hasn't been activated by that epoch
		proposedDealStartEpoch = deal.Proposal.StartEpoch

		// Not yet active, start matching against incoming messages
		return false, true, nil
	}

	// Watch for a pre-commit message to the provider.
	matchEvent := func(msg *types.Message) (bool, error) {
		matched := msg.To == provider && msg.Method == miner.Methods.PreCommitSector
		return matched, nil
	}

	// Check if the message params included the deal ID we're looking for.
	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(0, false, xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		// Check if waiting for pre-commit timed out
		if msg == nil {
			return false, xerrors.Errorf("timed out waiting for deal %d pre-commit", dealID)
		}

		// Extract the message parameters
		var params miner.SectorPreCommitInfo
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, xerrors.Errorf("unmarshal pre commit: %w", err)
		}

		// When the deal is published, the deal ID may change, so get the
		// current deal ID from the publish message CID
		dealID, _, err = GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
		if err != nil {
			return false, err
		}

		// Check through the deal IDs associated with this message
		for _, did := range params.DealIDs {
			if did == dealID {
				// Found the deal ID in this message. Callback with the sector ID.
				cb(params.SectorNumber, false, nil)
				return false, nil
			}
		}

		// Didn't find the deal ID in this message, so keep looking
		return true, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal pre-commit reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	if err := eventsApi.Called(checkFunc, called, revert, int(build.MessageConfidence+1), events.NoTimeout, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	// If the deal hasn't been activated by the proposed start epoch, timeout
	// the deal
	timeoutOnProposedStartEpoch(dealID, proposedDealStartEpoch, eventsApi, func(err error) {
		cb(0, false, err)
	})

	return nil
}

func OnDealSectorCommitted(ctx context.Context, api getCurrentDealInfoAPI, eventsApi sectorCommittedEventsAPI, provider address.Address, dealID abi.DealID, sectorNumber abi.SectorNumber, proposal market.DealProposal, publishCid *cid.Cid, callback storagemarket.DealSectorCommittedCallback) error {
	// Ensure callback is only called once
	var once sync.Once
	cb := func(err error) {
		once.Do(func() {
			callback(err)
		})
	}

	// First check if the deal is already active, and if so, bail out
	var proposedDealStartEpoch abi.ChainEpoch
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		deal, isActive, err := checkIfDealAlreadyActive(ctx, api, ts, dealID, proposal, publishCid)
		if err != nil {
			// Note: the error returned from here will end up being returned
			// from OnDealSectorCommitted so no need to call the callback
			// with the error
			return false, false, err
		}

		if isActive {
			// Deal is already active, bail out
			cb(nil)
			return true, false, nil
		}

		// Save the proposed deal start epoch so we can timeout if the deal
		// hasn't been activated by that epoch
		proposedDealStartEpoch = deal.Proposal.StartEpoch

		// Not yet active, start matching against incoming messages
		return false, true, nil
	}

	// Match a prove-commit sent to the provider with the given sector number
	matchEvent := func(msg *types.Message) (matched bool, err error) {
		if msg.To != provider || msg.Method != miner.Methods.ProveCommitSector {
			return false, nil
		}

		var params miner.ProveCommitSectorParams
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
		}

		return params.SectorNumber == sectorNumber, nil
	}

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		// Check if waiting for prove-commit timed out
		if msg == nil {
			return false, xerrors.Errorf("timed out waiting for deal activation for deal %d", dealID)
		}

		// Get the deal info
		_, sd, err := GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		// Make sure the deal is active
		if sd.State.SectorStartEpoch < 1 {
			return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", dealID, ts.ParentState(), ts.Height())
		}

		log.Infof("Storage deal %d activated at epoch %d", dealID, sd.State.SectorStartEpoch)

		cb(nil)

		return false, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	if err := eventsApi.Called(checkFunc, called, revert, int(build.MessageConfidence+1), events.NoTimeout, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	// If the deal hasn't been activated by the proposed start epoch, timeout
	// the deal
	timeoutOnProposedStartEpoch(dealID, proposedDealStartEpoch, eventsApi, func(err error) {
		cb(err)
	})

	return nil
}

func checkIfDealAlreadyActive(ctx context.Context, api getCurrentDealInfoAPI, ts *types.TipSet, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid) (*api.MarketDeal, bool, error) {
	_, sd, err := GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
	if err != nil {
		// TODO: This may be fine for some errors
		return nil, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
	}

	// Sector with deal is already active
	if sd.State.SectorStartEpoch > 0 {
		return nil, true, nil
	}

	// Sector was slashed
	if sd.State.SlashEpoch > 0 {
		return nil, false, xerrors.Errorf("deal %d was slashed at epoch %d", dealID, sd.State.SlashEpoch)
	}

	return sd, false, nil
}

// Once the chain reaches the proposed deal start epoch, callback with an error.
// Note that the functions that call timeoutOnProposedStartEpoch will ignore
// the callback if it's already been called (ie if a pre-commit or commit
// message lands on chain before the proposed deal start epoch).
func timeoutOnProposedStartEpoch(dealID abi.DealID, proposedDealStartEpoch abi.ChainEpoch, api sectorCommittedEventsAPI, cb func(err error)) {
	// Called when the chain height reaches deal start epoch + confidence
	heightAt := func(ctx context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		cb(xerrors.Errorf("deal %d was not activated by deal start epoch %d", dealID, proposedDealStartEpoch))
		return nil
	}

	// If the chain reorgs after reaching the deal start epoch, it's very
	// unlikely to reorg in such a way that the deal changes from
	// "not activated" to "activated before deal start epoch", so just log a
	// warning.
	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warnf("deal %d had reached start epoch %d but the chain reorged", dealID, proposedDealStartEpoch)
		return nil
	}

	err := api.ChainAt(heightAt, revert, int(build.MessageConfidence+1), proposedDealStartEpoch+1)
	if err != nil {
		cb(xerrors.Errorf("error waiting for deal %d to become activated: %w", dealID, err))
	}
}
