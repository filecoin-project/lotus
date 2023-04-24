package providerstates

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// ProviderEvents are the events that can happen in a storage provider
var ProviderEvents = fsm.Events{
	fsm.Event(storagemarket.ProviderEventOpen).From(storagemarket.StorageDealUnknown).To(storagemarket.StorageDealValidating),
	fsm.Event(storagemarket.ProviderEventNodeErrored).FromAny().To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error calling node: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealRejected).
		FromMany(storagemarket.StorageDealValidating, storagemarket.StorageDealVerifyData, storagemarket.StorageDealAcceptWait).To(storagemarket.StorageDealRejecting).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("deal rejected: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventRejectionSent).
		From(storagemarket.StorageDealRejecting).To(storagemarket.StorageDealFailing),
	fsm.Event(storagemarket.ProviderEventDealDeciding).
		From(storagemarket.StorageDealValidating).To(storagemarket.StorageDealAcceptWait),
	fsm.Event(storagemarket.ProviderEventDataRequested).
		From(storagemarket.StorageDealAcceptWait).To(storagemarket.StorageDealWaitingForData),

	fsm.Event(storagemarket.ProviderEventDataTransferFailed).
		FromMany(storagemarket.StorageDealTransferring, storagemarket.StorageDealProviderTransferAwaitRestart).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error transferring data: %w", err).Error()
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventDataTransferInitiated).
		FromMany(storagemarket.StorageDealWaitingForData, storagemarket.StorageDealProviderTransferAwaitRestart).
		To(storagemarket.StorageDealTransferring).
		Action(func(deal *storagemarket.MinerDeal, channelId datatransfer.ChannelID) error {
			deal.TransferChannelId = &channelId
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventDataTransferRestarted).
		FromMany(storagemarket.StorageDealWaitingForData, storagemarket.StorageDealProviderTransferAwaitRestart).
		To(storagemarket.StorageDealTransferring).
		From(storagemarket.StorageDealTransferring).ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal, channelId datatransfer.ChannelID) error {
			deal.TransferChannelId = &channelId
			deal.Message = ""
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventDataTransferStalled).
		FromMany(storagemarket.StorageDealTransferring, storagemarket.StorageDealProviderTransferAwaitRestart).
		ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal) error {
			deal.Message = "data transfer appears to be stalled, awaiting reconnect from client"
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventDataTransferCancelled).
		FromMany(
			storagemarket.StorageDealWaitingForData,
			storagemarket.StorageDealTransferring,
			storagemarket.StorageDealProviderTransferAwaitRestart,
		).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal) error {
			deal.Message = "data transfer cancelled"
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventDataTransferCompleted).
		FromMany(storagemarket.StorageDealTransferring, storagemarket.StorageDealProviderTransferAwaitRestart).
		To(storagemarket.StorageDealVerifyData),

	fsm.Event(storagemarket.ProviderEventDataVerificationFailed).
		From(storagemarket.StorageDealVerifyData).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error, path filestore.Path, metadataPath filestore.Path) error {
			deal.PiecePath = path
			deal.MetadataPath = metadataPath
			deal.Message = xerrors.Errorf("deal data verification failed: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventVerifiedData).
		FromMany(storagemarket.StorageDealVerifyData, storagemarket.StorageDealWaitingForData).To(storagemarket.StorageDealReserveProviderFunds).
		Action(func(deal *storagemarket.MinerDeal, path filestore.Path, metadataPath filestore.Path) error {
			deal.PiecePath = path
			deal.MetadataPath = metadataPath
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventFundingInitiated).
		From(storagemarket.StorageDealReserveProviderFunds).To(storagemarket.StorageDealProviderFunding).
		Action(func(deal *storagemarket.MinerDeal, mcid cid.Cid) error {
			deal.AddFundsCid = &mcid
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventFunded).
		FromMany(storagemarket.StorageDealProviderFunding, storagemarket.StorageDealReserveProviderFunds).To(storagemarket.StorageDealPublish),
	fsm.Event(storagemarket.ProviderEventDealPublishInitiated).
		From(storagemarket.StorageDealPublish).To(storagemarket.StorageDealPublishing).
		Action(func(deal *storagemarket.MinerDeal, finalCid cid.Cid) error {
			deal.PublishCid = &finalCid
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealPublishError).
		From(storagemarket.StorageDealPublishing).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("PublishStorageDeal error: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventSendResponseFailed).
		FromMany(storagemarket.StorageDealAcceptWait, storagemarket.StorageDealRejecting).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("sending response to deal: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealPublished).
		From(storagemarket.StorageDealPublishing).To(storagemarket.StorageDealStaged).
		Action(func(deal *storagemarket.MinerDeal, dealID abi.DealID, finalCid cid.Cid) error {
			deal.DealID = dealID
			deal.PublishCid = &finalCid
			return nil
		}),
	// add by lin
	fsm.Event(storagemarket.ProviderEventDealPublishedOfSxx).
		From(storagemarket.StorageDealPublishing).To(storagemarket.StorageDealStagedOfSxx).
		Action(func(deal *storagemarket.MinerDeal, dealID abi.DealID, finalCid cid.Cid) error {
			deal.DealID = dealID
			deal.PublishCid = &finalCid
			return nil
		}),
	// end
	// change by lin
	fsm.Event(storagemarket.ProviderEventFileStoreErrored).
		FromMany(storagemarket.StorageDealStaged, storagemarket.StorageDealStagedOfSxx, storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing, storagemarket.StorageDealActive).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("accessing file store: %w", err).Error()
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventMultistoreErrored).
		FromMany(storagemarket.StorageDealStaged, storagemarket.StorageDealStagedOfSxx).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("operating on multistore: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealHandoffFailed).FromMany(storagemarket.StorageDealStaged, storagemarket.StorageDealStagedOfSxx).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("handing off deal to node: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventPieceStoreErrored).
		FromMany(storagemarket.StorageDealStaged, storagemarket.StorageDealStagedOfSxx).ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("recording piece for retrieval: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealHandedOff).
		FromMany(storagemarket.StorageDealStaged, storagemarket.StorageDealStagedOfSxx).To(storagemarket.StorageDealAwaitingPreCommit).
		Action(func(deal *storagemarket.MinerDeal) error {
			deal.AvailableForRetrieval = true
			return nil
		}),
	// end
	fsm.Event(storagemarket.ProviderEventDealPrecommitFailed).
		From(storagemarket.StorageDealAwaitingPreCommit).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error awaiting deal pre-commit: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealPrecommitted).
		From(storagemarket.StorageDealAwaitingPreCommit).To(storagemarket.StorageDealSealing).
		Action(func(deal *storagemarket.MinerDeal, sectorNumber abi.SectorNumber) error {
			deal.SectorNumber = sectorNumber
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealActivationFailed).
		From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error activating deal: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealActivated).
		FromMany(storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing).
		To(storagemarket.StorageDealFinalizing),
	fsm.Event(storagemarket.ProviderEventFinalized).
		From(storagemarket.StorageDealFinalizing).To(storagemarket.StorageDealActive).
		Action(func(deal *storagemarket.MinerDeal) error {
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealSlashed).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealSlashed).
		Action(func(deal *storagemarket.MinerDeal, slashEpoch abi.ChainEpoch) error {
			deal.SlashEpoch = slashEpoch
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventDealExpired).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealExpired),
	fsm.Event(storagemarket.ProviderEventDealCompletionFailed).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error waiting for deal completion: %w", err).Error()
			return nil
		}),

	fsm.Event(storagemarket.ProviderEventFailed).From(storagemarket.StorageDealFailing).To(storagemarket.StorageDealError),

	fsm.Event(storagemarket.ProviderEventRestart).
		FromMany(storagemarket.StorageDealValidating, storagemarket.StorageDealAcceptWait, storagemarket.StorageDealRejecting).
		To(storagemarket.StorageDealError).
		From(storagemarket.StorageDealTransferring).
		To(storagemarket.StorageDealProviderTransferAwaitRestart).
		FromAny().ToNoChange(),

	fsm.Event(storagemarket.ProviderEventAwaitTransferRestartTimeout).
		From(storagemarket.StorageDealProviderTransferAwaitRestart).To(storagemarket.StorageDealFailing).
		FromAny().ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal) error {
			if deal.State == storagemarket.StorageDealProviderTransferAwaitRestart {
				deal.Message = fmt.Sprintf("timed out waiting for client to restart transfer")
			}
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventTrackFundsFailed).
		From(storagemarket.StorageDealReserveProviderFunds).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.MinerDeal, err error) error {
			deal.Message = xerrors.Errorf("error tracking deal funds: %w", err).Error()
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventFundsReserved).
		From(storagemarket.StorageDealReserveProviderFunds).ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal, fundsReserved abi.TokenAmount) error {
			if deal.FundsReserved.Nil() {
				deal.FundsReserved = fundsReserved
			} else {
				deal.FundsReserved = big.Add(deal.FundsReserved, fundsReserved)
			}
			return nil
		}),
	fsm.Event(storagemarket.ProviderEventFundsReleased).
		FromMany(storagemarket.StorageDealPublishing, storagemarket.StorageDealFailing).ToJustRecord().
		Action(func(deal *storagemarket.MinerDeal, fundsReleased abi.TokenAmount) error {
			deal.FundsReserved = big.Subtract(deal.FundsReserved, fundsReleased)
			return nil
		}),
}

// ProviderStateEntryFuncs are the handlers for different states in a storage client
var ProviderStateEntryFuncs = fsm.StateEntryFuncs{
	storagemarket.StorageDealValidating:                   ValidateDealProposal,
	storagemarket.StorageDealAcceptWait:                   DecideOnProposal,
	storagemarket.StorageDealProviderTransferAwaitRestart: WaitForTransferRestart,
	storagemarket.StorageDealVerifyData:                   VerifyData,
	storagemarket.StorageDealReserveProviderFunds:         ReserveProviderFunds,
	storagemarket.StorageDealProviderFunding:              WaitForFunding,
	storagemarket.StorageDealPublish:                      PublishDeal,
	storagemarket.StorageDealPublishing:                   WaitForPublish,
	storagemarket.StorageDealStaged:                       HandoffDeal,
	// add by lin
	storagemarket.StorageDealStagedOfSxx:                  HandoffDealOfSxx,
	// end
	storagemarket.StorageDealAwaitingPreCommit:            VerifyDealPreCommitted,
	storagemarket.StorageDealSealing:                      VerifyDealActivated,
	storagemarket.StorageDealRejecting:                    RejectDeal,
	storagemarket.StorageDealFinalizing:                   CleanupDeal,
	storagemarket.StorageDealActive:                       WaitForDealCompletion,
	storagemarket.StorageDealFailing:                      FailDeal,
}

// ProviderFinalityStates are the states that terminate deal processing for a deal.
// When a provider restarts, it restarts only deals that are not in a finality state.
var ProviderFinalityStates = []fsm.StateKey{
	storagemarket.StorageDealError,
	storagemarket.StorageDealSlashed,
	storagemarket.StorageDealExpired,
}

// StatesKnownBySealingSubsystem are the states on the happy path after hand-off to
// the sealing subsystem
var StatesKnownBySealingSubsystem = []fsm.StateKey{
	storagemarket.StorageDealAwaitingPreCommit,
	storagemarket.StorageDealSealing,
	storagemarket.StorageDealFinalizing,
	storagemarket.StorageDealActive,
}
