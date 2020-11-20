package storageadapter

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
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
}

type sectorPreCommitAPI interface {
	getCurrentDealInfoAPI
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
}

func OnDealSectorPreCommitted(ctx context.Context, api sectorPreCommitAPI, eventsApi sectorCommittedEventsAPI, provider address.Address, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorPreCommittedCallback) error {
	// First check if the deal is already active, and if so, bail out
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		isActive, err := checkIfDealAlreadyActive(ctx, api, ts, dealID, proposal, publishCid)
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

		// Not yet active, start matching against incoming messages
		return false, true, nil
	}

	// Watch for a pre-commit or prove-commit message to the provider.
	// It's possible (when the node restarts) that the pre-commit has already
	// been submitted, in which case we wait for the prove-commit message,
	// so match both pre-commit and prove-commit messages.
	matchEvent := func(msg *types.Message) (matched bool, err error) {
		if msg.To != provider {
			return false, nil
		}

		switch msg.Method {
		case miner.Methods.PreCommitSector:
			return true, nil
		case miner.Methods.ProveCommitSector:
			return true, nil
		}

		return false, nil
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

		var sectorNumber abi.SectorNumber
		var msgDealIDs []abi.DealID
		if msg.Method == miner.Methods.PreCommitSector {
			// If it's a pre-commit message, the deal IDs are in the message parameters
			var params miner.SectorPreCommitInfo
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("unmarshal pre commit: %w", err)
			}
			msgDealIDs = params.DealIDs
			sectorNumber = params.SectorNumber
		} else {
			// If it's a prove-commit message, the parameters don't have deal IDs,
			// just a sector number
			var params miner.ProveCommitSectorParams
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
			}

			// Look up the sector number in miner state to get the deal IDs
			sectorNumber = params.SectorNumber
			sectorInfo, err := api.StateSectorGetInfo(ctx, provider, sectorNumber, ts.Key())
			if err != nil {
				return false, xerrors.Errorf("failed to get sector info for sector %d: %w", sectorNumber, err)
			}

			// If there is no sector info for this sector, ignore it
			if sectorInfo == nil {
				return true, nil
			}

			msgDealIDs = sectorInfo.DealIDs
		}

		// When the deal is published, the deal ID may change, so get the
		// current deal ID from the publish message CID
		dealID, _, err = GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
		if err != nil {
			return false, err
		}

		// Check through the deal IDs associated with this message
		for _, did := range msgDealIDs {
			if did == dealID {
				// Found the deal ID in this message. Callback with the sector ID.
				// If the message is a prove-commit, then the sector is already active.
				isActive := msg.Method == miner.Methods.ProveCommitSector
				cb(sectorNumber, isActive, nil)
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

	return nil
}

func OnDealSectorCommitted(ctx context.Context, api getCurrentDealInfoAPI, eventsApi sectorCommittedEventsAPI, provider address.Address, dealID abi.DealID, sectorNumber abi.SectorNumber, proposal market.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorCommittedCallback) error {
	// First check if the deal is already active, and if so, bail out
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		isActive, err := checkIfDealAlreadyActive(ctx, api, ts, dealID, proposal, publishCid)
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

	return nil
}

func checkIfDealAlreadyActive(ctx context.Context, api getCurrentDealInfoAPI, ts *types.TipSet, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid) (bool, error) {
	_, sd, err := GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
	if err != nil {
		// TODO: This may be fine for some errors
		return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
	}

	// Sector with deal is already active
	isActive := sd.State.SectorStartEpoch > 0
	return isActive, nil
}
