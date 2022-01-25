package storageadapter

import (
	"bytes"
	"context"
	"sync"

	"github.com/filecoin-project/go-bitfield"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
)

type eventsCalledAPI interface {
	Called(ctx context.Context, check events.CheckFunc, msgHnd events.MsgHandler, rev events.RevertHandler, confidence int, timeout abi.ChainEpoch, mf events.MsgMatchFunc) error
}

type dealInfoAPI interface {
	GetCurrentDealInfo(ctx context.Context, tok sealing.TipSetToken, proposal *market.DealProposal, publishCid cid.Cid) (sealing.CurrentDealInfo, error)
}

type diffPreCommitsAPI interface {
	diffPreCommits(ctx context.Context, actor address.Address, pre, cur types.TipSetKey) (*miner.PreCommitChanges, error)
}

type SectorCommittedManager struct {
	ev       eventsCalledAPI
	dealInfo dealInfoAPI
	dpc      diffPreCommitsAPI
}

func NewSectorCommittedManager(ev eventsCalledAPI, tskAPI sealing.CurrentDealInfoTskAPI, dpcAPI diffPreCommitsAPI) *SectorCommittedManager {
	dim := &sealing.CurrentDealInfoManager{
		CDAPI: &sealing.CurrentDealInfoAPIAdapter{CurrentDealInfoTskAPI: tskAPI},
	}
	return newSectorCommittedManager(ev, dim, dpcAPI)
}

func newSectorCommittedManager(ev eventsCalledAPI, dealInfo dealInfoAPI, dpcAPI diffPreCommitsAPI) *SectorCommittedManager {
	return &SectorCommittedManager{
		ev:       ev,
		dealInfo: dealInfo,
		dpc:      dpcAPI,
	}
}

func (mgr *SectorCommittedManager) OnDealSectorPreCommitted(ctx context.Context, provider address.Address, proposal market.DealProposal, publishCid cid.Cid, callback storagemarket.DealSectorPreCommittedCallback) error {
	// Ensure callback is only called once
	var once sync.Once
	cb := func(sectorNumber abi.SectorNumber, isActive bool, err error) {
		once.Do(func() {
			callback(sectorNumber, isActive, err)
		})
	}

	// First check if the deal is already active, and if so, bail out
	checkFunc := func(ctx context.Context, ts *types.TipSet) (done bool, more bool, err error) {
		dealInfo, isActive, err := mgr.checkIfDealAlreadyActive(ctx, ts, &proposal, publishCid)
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

		// Check that precommits which landed between when the deal was published
		// and now don't already contain the deal we care about.
		// (this can happen when the precommit lands vary quickly (in tests), or
		// when the client node was down after the deal was published, and when
		// the precommit containing it landed on chain)

		publishTs, err := types.TipSetKeyFromBytes(dealInfo.PublishMsgTipSet)
		if err != nil {
			return false, false, err
		}

		diff, err := mgr.dpc.diffPreCommits(ctx, provider, publishTs, ts.Key())
		if err != nil {
			return false, false, err
		}

		for _, info := range diff.Added {
			for _, d := range info.Info.DealIDs {
				if d == dealInfo.DealID {
					cb(info.Info.SectorNumber, false, nil)
					return true, false, nil
				}
			}
		}

		// Not yet active, start matching against incoming messages
		return false, true, nil
	}

	// Watch for a pre-commit message to the provider.
	matchEvent := func(msg *types.Message) (bool, error) {
		matched := msg.To == provider && (msg.Method == miner.Methods.PreCommitSector || msg.Method == miner.Methods.PreCommitSectorBatch || msg.Method == miner.Methods.ProveReplicaUpdates)
		return matched, nil
	}

	// The deal must be accepted by the deal proposal start epoch, so timeout
	// if the chain reaches that epoch
	timeoutEpoch := proposal.StartEpoch + 1

	// Check if the message params included the deal ID we're looking for.
	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(0, false, xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		// If the deal hasn't been activated by the proposed start epoch, the
		// deal will timeout (when msg == nil it means the timeout epoch was reached)
		if msg == nil {
			err = xerrors.Errorf("deal with piece CID %s was not activated by proposed deal start epoch %d", proposal.PieceCID, proposal.StartEpoch)
			return false, err
		}

		// Ignore the pre-commit message if it was not executed successfully
		if rec.ExitCode != 0 {
			return true, nil
		}

		// When there is a reorg, the deal ID may change, so get the
		// current deal ID from the publish message CID
		res, err := mgr.dealInfo.GetCurrentDealInfo(ctx, ts.Key().Bytes(), &proposal, publishCid)
		if err != nil {
			return false, err
		}

		// If this is a replica update method that succeeded the deal is active
		if msg.Method == miner.Methods.ProveReplicaUpdates {
			sn, err := dealSectorInReplicaUpdateSuccess(msg, rec, res)
			if err != nil {
				return false, err
			}
			if sn != nil {
				cb(*sn, true, nil)
				return false, nil
			}
			// Didn't find the deal ID in this message, so keep looking
			return true, nil
		}

		// Extract the message parameters
		sn, err := dealSectorInPreCommitMsg(msg, res)
		if err != nil {
			return false, err
		}

		if sn != nil {
			cb(*sn, false, nil)
		}

		// Didn't find the deal ID in this message, so keep looking
		return true, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal pre-commit reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	if err := mgr.ev.Called(ctx, checkFunc, called, revert, int(build.MessageConfidence+1), timeoutEpoch, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	return nil
}

func (mgr *SectorCommittedManager) OnDealSectorCommitted(ctx context.Context, provider address.Address, sectorNumber abi.SectorNumber, proposal market.DealProposal, publishCid cid.Cid, callback storagemarket.DealSectorCommittedCallback) error {
	// Ensure callback is only called once
	var once sync.Once
	cb := func(err error) {
		once.Do(func() {
			callback(err)
		})
	}

	// First check if the deal is already active, and if so, bail out
	checkFunc := func(ctx context.Context, ts *types.TipSet) (done bool, more bool, err error) {
		_, isActive, err := mgr.checkIfDealAlreadyActive(ctx, ts, &proposal, publishCid)
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
		if msg.To != provider {
			return false, nil
		}

		return sectorInCommitMsg(msg, sectorNumber)
	}

	// The deal must be accepted by the deal proposal start epoch, so timeout
	// if the chain reaches that epoch
	timeoutEpoch := proposal.StartEpoch + 1

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		// If the deal hasn't been activated by the proposed start epoch, the
		// deal will timeout (when msg == nil it means the timeout epoch was reached)
		if msg == nil {
			err := xerrors.Errorf("deal with piece CID %s was not activated by proposed deal start epoch %d", proposal.PieceCID, proposal.StartEpoch)
			return false, err
		}

		// Ignore the prove-commit message if it was not executed successfully
		if rec.ExitCode != 0 {
			return true, nil
		}

		// Get the deal info
		res, err := mgr.dealInfo.GetCurrentDealInfo(ctx, ts.Key().Bytes(), &proposal, publishCid)
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		// Make sure the deal is active
		if res.MarketDeal.State.SectorStartEpoch < 1 {
			return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", res.DealID, ts.ParentState(), ts.Height())
		}

		log.Infof("Storage deal %d activated at epoch %d", res.DealID, res.MarketDeal.State.SectorStartEpoch)

		cb(nil)

		return false, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	if err := mgr.ev.Called(ctx, checkFunc, called, revert, int(build.MessageConfidence+1), timeoutEpoch, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	return nil
}

func dealSectorInReplicaUpdateSuccess(msg *types.Message, rec *types.MessageReceipt, res sealing.CurrentDealInfo) (*abi.SectorNumber, error) {
	var params miner.ProveReplicaUpdatesParams
	if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
		return nil, xerrors.Errorf("unmarshal prove replica update: %w", err)
	}

	var seekUpdate miner.ReplicaUpdate
	var found bool
	for _, update := range params.Updates {
		for _, did := range update.Deals {
			if did == res.DealID {
				seekUpdate = update
				found = true
				break
			}
		}
	}
	if !found {
		return nil, nil
	}

	// check that this update passed validation steps
	var successBf bitfield.BitField
	if err := successBf.UnmarshalCBOR(bytes.NewReader(rec.Return)); err != nil {
		return nil, xerrors.Errorf("unmarshal return value: %w", err)
	}
	success, err := successBf.IsSet(uint64(seekUpdate.SectorID))
	if err != nil {
		return nil, xerrors.Errorf("failed to check success of replica update: %w", err)
	}
	if !success {
		return nil, xerrors.Errorf("replica update %d failed", seekUpdate.SectorID)
	}
	return &seekUpdate.SectorID, nil
}

// dealSectorInPreCommitMsg tries to find a sector containing the specified deal
func dealSectorInPreCommitMsg(msg *types.Message, res sealing.CurrentDealInfo) (*abi.SectorNumber, error) {
	switch msg.Method {
	case miner.Methods.PreCommitSector:
		var params miner.SectorPreCommitInfo
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return nil, xerrors.Errorf("unmarshal pre commit: %w", err)
		}

		// Check through the deal IDs associated with this message
		for _, did := range params.DealIDs {
			if did == res.DealID {
				// Found the deal ID in this message. Callback with the sector ID.
				return &params.SectorNumber, nil
			}
		}
	case miner.Methods.PreCommitSectorBatch:
		var params miner5.PreCommitSectorBatchParams
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return nil, xerrors.Errorf("unmarshal pre commit: %w", err)
		}

		for _, precommit := range params.Sectors {
			// Check through the deal IDs associated with this message
			for _, did := range precommit.DealIDs {
				if did == res.DealID {
					// Found the deal ID in this message. Callback with the sector ID.
					return &precommit.SectorNumber, nil
				}
			}
		}
	default:
		return nil, xerrors.Errorf("unexpected method %d", msg.Method)
	}

	return nil, nil
}

// sectorInCommitMsg checks if the provided message commits specified sector
func sectorInCommitMsg(msg *types.Message, sectorNumber abi.SectorNumber) (bool, error) {
	switch msg.Method {
	case miner.Methods.ProveCommitSector:
		var params miner.ProveCommitSectorParams
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
		}

		return params.SectorNumber == sectorNumber, nil

	case miner.Methods.ProveCommitAggregate:
		var params miner5.ProveCommitAggregateParams
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
		}

		set, err := params.SectorNumbers.IsSet(uint64(sectorNumber))
		if err != nil {
			return false, xerrors.Errorf("checking if sectorNumber is set in commit aggregate message: %w", err)
		}

		return set, nil

	default:
		return false, nil
	}
}

func (mgr *SectorCommittedManager) checkIfDealAlreadyActive(ctx context.Context, ts *types.TipSet, proposal *market.DealProposal, publishCid cid.Cid) (sealing.CurrentDealInfo, bool, error) {
	res, err := mgr.dealInfo.GetCurrentDealInfo(ctx, ts.Key().Bytes(), proposal, publishCid)
	if err != nil {
		// TODO: This may be fine for some errors
		return res, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
	}

	// Sector was slashed
	if res.MarketDeal.State.SlashEpoch > 0 {
		return res, false, xerrors.Errorf("deal %d was slashed at epoch %d", res.DealID, res.MarketDeal.State.SlashEpoch)
	}

	// Sector with deal is already active
	if res.MarketDeal.State.SectorStartEpoch > 0 {
		return res, true, nil
	}

	return res, false, nil
}
