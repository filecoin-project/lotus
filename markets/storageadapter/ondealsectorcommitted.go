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

func OnDealSectorCommitted(ctx context.Context, api getCurrentDealInfoAPI, eventsApi sectorCommittedEventsAPI, provider address.Address, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorCommittedCallback) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		newDealID, sd, err := GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
		if err != nil {
			// TODO: This may be fine for some errors
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}
		dealID = newDealID

		if sd.State.SectorStartEpoch > 0 {
			cb(nil)
			return true, false, nil
		}

		return false, true, nil
	}

	var sectorNumber abi.SectorNumber
	var sectorFound bool

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(xerrors.Errorf("handling applied event: %w", err))
			}
		}()
		switch msg.Method {
		case miner.Methods.PreCommitSector:
			var params miner.SectorPreCommitInfo
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("unmarshal pre commit: %w", err)
			}

			dealID, _, err = GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
			if err != nil {
				return false, err
			}

			for _, did := range params.DealIDs {
				if did == dealID {
					sectorNumber = params.SectorNumber
					sectorFound = true
					return true, nil
				}
			}
			return true, nil
		case miner.Methods.ProveCommitSector:
			if msg == nil {
				log.Error("timed out waiting for deal activation... what now?")
				return false, nil
			}

			_, sd, err := GetCurrentDealInfo(ctx, ts, api, dealID, proposal, publishCid)
			if err != nil {
				return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
			}

			if sd.State.SectorStartEpoch < 1 {
				return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", dealID, ts.ParentState(), ts.Height())
			}

			log.Infof("Storage deal %d activated at epoch %d", dealID, sd.State.SectorStartEpoch)

			cb(nil)

			return false, nil
		default:
			return false, nil
		}
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	matchEvent := func(msg *types.Message) (matched bool, err error) {
		if msg.To != provider {
			return false, nil
		}

		switch msg.Method {
		case miner.Methods.PreCommitSector:
			return !sectorFound, nil
		case miner.Methods.ProveCommitSector:
			if !sectorFound {
				return false, nil
			}

			var params miner.ProveCommitSectorParams
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
			}

			if params.SectorNumber != sectorNumber {
				return false, nil
			}

			return true, nil
		default:
			return false, nil
		}

	}

	if err := eventsApi.Called(checkFunc, called, revert, int(build.MessageConfidence+1), events.NoTimeout, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	return nil
}
