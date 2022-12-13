package clientstates

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// ClientEvents are the events that can happen in a storage client
var ClientEvents = fsm.Events{
	fsm.Event(storagemarket.ClientEventOpen).
		From(storagemarket.StorageDealUnknown).To(storagemarket.StorageDealReserveClientFunds),
	fsm.Event(storagemarket.ClientEventFundingInitiated).
		From(storagemarket.StorageDealReserveClientFunds).To(storagemarket.StorageDealClientFunding).
		Action(func(deal *storagemarket.ClientDeal, mcid cid.Cid) error {
			deal.AddFundsCid = &mcid
			deal.AddLog("reserving funds for storage deal, message cid: <%s>", mcid)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventReserveFundsFailed).
		FromMany(storagemarket.StorageDealClientFunding, storagemarket.StorageDealReserveClientFunds).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("adding market funds failed: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventFundsReserved).
		From(storagemarket.StorageDealReserveClientFunds).ToJustRecord().
		Action(func(deal *storagemarket.ClientDeal, fundsReserved abi.TokenAmount) error {
			if deal.FundsReserved.Nil() {
				deal.FundsReserved = fundsReserved
			} else {
				deal.FundsReserved = big.Add(deal.FundsReserved, fundsReserved)
			}
			deal.AddLog("funds reserved, amount <%s>", fundsReserved)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventFundsReleased).
		FromMany(storagemarket.StorageDealProposalAccepted, storagemarket.StorageDealFailing).ToJustRecord().
		Action(func(deal *storagemarket.ClientDeal, fundsReleased abi.TokenAmount) error {
			deal.FundsReserved = big.Subtract(deal.FundsReserved, fundsReleased)
			deal.AddLog("funds released, amount <%s>", fundsReleased)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventFundingComplete).
		FromMany(storagemarket.StorageDealReserveClientFunds, storagemarket.StorageDealClientFunding).To(storagemarket.StorageDealFundsReserved),
	fsm.Event(storagemarket.ClientEventWriteProposalFailed).
		From(storagemarket.StorageDealFundsReserved).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("sending proposal to storage provider failed: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventReadResponseFailed).
		From(storagemarket.StorageDealFundsReserved).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error reading Response message from provider: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventResponseVerificationFailed).
		From(storagemarket.StorageDealFundsReserved).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.Message = "unable to verify signature on deal response"
			deal.AddLog(deal.Message)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventInitiateDataTransfer).
		From(storagemarket.StorageDealFundsReserved).To(storagemarket.StorageDealStartDataTransfer).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.AddLog("opening data transfer to storage provider")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventUnexpectedDealState).
		From(storagemarket.StorageDealFundsReserved).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, status storagemarket.StorageDealStatus, providerMessage string) error {
			deal.Message = xerrors.Errorf("unexpected deal status while waiting for data request: %d (%s). Provider message: %s", status, storagemarket.DealStates[status], providerMessage).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDataTransferFailed).
		FromMany(storagemarket.StorageDealStartDataTransfer, storagemarket.StorageDealTransferring, storagemarket.StorageDealTransferQueued).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("failed to complete data transfer: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferRestartFailed).From(storagemarket.StorageDealClientTransferRestart).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("failed to restart data transfer: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),

	// The client has sent a push request to the provider, and in response the provider has
	// opened a request for data to the client. The transfer is in the client's queue.
	fsm.Event(storagemarket.ClientEventDataTransferQueued).
		FromMany(storagemarket.StorageDealStartDataTransfer).To(storagemarket.StorageDealTransferQueued).
		Action(func(deal *storagemarket.ClientDeal, channelId datatransfer.ChannelID) error {
			deal.AddLog("provider data transfer request added to client's queue: channel id <%s>", channelId)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferInitiated).
		FromMany(storagemarket.StorageDealTransferQueued).To(storagemarket.StorageDealTransferring).
		Action(func(deal *storagemarket.ClientDeal, channelId datatransfer.ChannelID) error {
			deal.TransferChannelID = &channelId
			deal.AddLog("data transfer initiated on channel id <%s>", channelId)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferRestarted).
		FromMany(storagemarket.StorageDealClientTransferRestart, storagemarket.StorageDealStartDataTransfer, storagemarket.StorageDealTransferQueued).To(storagemarket.StorageDealTransferring).
		From(storagemarket.StorageDealTransferring).ToJustRecord().
		Action(func(deal *storagemarket.ClientDeal, channelId datatransfer.ChannelID) error {
			deal.TransferChannelID = &channelId
			deal.Message = ""
			deal.AddLog("data transfer restarted on channel id <%s>", channelId)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferStalled).
		FromMany(storagemarket.StorageDealTransferring, storagemarket.StorageDealTransferQueued).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("could not complete data transfer, could not connect to provider %s", deal.Miner).Error()
			deal.AddLog(deal.Message)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferCancelled).
		FromMany(
			storagemarket.StorageDealStartDataTransfer,
			storagemarket.StorageDealTransferring,
			storagemarket.StorageDealClientTransferRestart,
			storagemarket.StorageDealTransferQueued,
		).
		To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.Message = "data transfer cancelled"
			deal.AddLog(deal.Message)
			return nil
		}),

	fsm.Event(storagemarket.ClientEventDataTransferComplete).
		FromMany(storagemarket.StorageDealTransferring, storagemarket.StorageDealStartDataTransfer, storagemarket.StorageDealTransferQueued).
		To(storagemarket.StorageDealCheckForAcceptance),
	fsm.Event(storagemarket.ClientEventWaitForDealState).
		From(storagemarket.StorageDealCheckForAcceptance).ToNoChange().
		Action(func(deal *storagemarket.ClientDeal, pollError bool, providerState storagemarket.StorageDealStatus) error {
			deal.PollRetryCount++
			if pollError {
				deal.PollErrorCount++
			}
			deal.Message = fmt.Sprintf("Provider state: %s", storagemarket.DealStates[providerState])
			switch storagemarket.DealStates[providerState] {
			case "StorageDealVerifyData":
				deal.AddLog("provider is verifying the data")
			case "StorageDealPublish":
				deal.AddLog("waiting for provider to publish the deal on-chain") // TODO: is that right?
			case "StorageDealPublishing":
				deal.AddLog("provider has submitted the deal on-chain and is waiting for confirmation") // TODO: is that right?
			case "StorageDealProviderFunding":
				deal.AddLog("waiting for provider to lock collateral on-chain") // TODO: is that right?
			default:
				deal.AddLog(deal.Message)
			}
			return nil
		}),
	fsm.Event(storagemarket.ClientEventResponseDealDidNotMatch).
		From(storagemarket.StorageDealCheckForAcceptance).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, responseCid cid.Cid, proposalCid cid.Cid) error {
			deal.Message = xerrors.Errorf("miner responded to a wrong proposal: %s != %s", responseCid, proposalCid).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealRejected).
		From(storagemarket.StorageDealCheckForAcceptance).To(storagemarket.StorageDealFailing).
		Action(func(deal *storagemarket.ClientDeal, state storagemarket.StorageDealStatus, reason string) error {
			deal.Message = xerrors.Errorf("deal failed: (State=%d) %s", state, reason).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealAccepted).
		From(storagemarket.StorageDealCheckForAcceptance).To(storagemarket.StorageDealProposalAccepted).
		Action(func(deal *storagemarket.ClientDeal, publishMessage *cid.Cid) error {
			deal.PublishMessage = publishMessage
			deal.Message = ""
			deal.AddLog("deal has been accepted by storage provider")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventStreamCloseError).
		FromAny().To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error attempting to close stream: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPublishFailed).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error validating deal published: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPublished).
		From(storagemarket.StorageDealProposalAccepted).To(storagemarket.StorageDealAwaitingPreCommit).
		Action(func(deal *storagemarket.ClientDeal, dealID abi.DealID) error {
			deal.DealID = dealID
			deal.AddLog("")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPrecommitFailed).
		From(storagemarket.StorageDealAwaitingPreCommit).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error waiting for deal pre-commit message to appear on chain: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealPrecommitted).
		From(storagemarket.StorageDealAwaitingPreCommit).To(storagemarket.StorageDealSealing).
		Action(func(deal *storagemarket.ClientDeal, sectorNumber abi.SectorNumber) error {
			deal.SectorNumber = sectorNumber
			deal.AddLog("deal pre-commit message has landed on chain")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealActivationFailed).
		From(storagemarket.StorageDealSealing).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error in deal activation: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealActivated).
		FromMany(storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing).
		To(storagemarket.StorageDealActive).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.AddLog("deal activated")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealSlashed).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealSlashed).
		Action(func(deal *storagemarket.ClientDeal, slashEpoch abi.ChainEpoch) error {
			deal.SlashEpoch = slashEpoch
			deal.AddLog("deal slashed at epoch <%d>", slashEpoch)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventDealExpired).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealExpired),
	fsm.Event(storagemarket.ClientEventDealCompletionFailed).
		From(storagemarket.StorageDealActive).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal, err error) error {
			deal.Message = xerrors.Errorf("error waiting for deal completion: %w", err).Error()
			deal.AddLog(deal.Message)
			return nil
		}),
	fsm.Event(storagemarket.ClientEventFailed).
		From(storagemarket.StorageDealFailing).To(storagemarket.StorageDealError).
		Action(func(deal *storagemarket.ClientDeal) error {
			deal.AddLog("")
			return nil
		}),
	fsm.Event(storagemarket.ClientEventRestart).From(storagemarket.StorageDealTransferring).To(storagemarket.StorageDealClientTransferRestart).
		FromAny().ToNoChange(),
}

// ClientStateEntryFuncs are the handlers for different states in a storage client
var ClientStateEntryFuncs = fsm.StateEntryFuncs{
	storagemarket.StorageDealReserveClientFunds:    ReserveClientFunds,
	storagemarket.StorageDealClientFunding:         WaitForFunding,
	storagemarket.StorageDealFundsReserved:         ProposeDeal,
	storagemarket.StorageDealStartDataTransfer:     InitiateDataTransfer,
	storagemarket.StorageDealClientTransferRestart: RestartDataTransfer,
	storagemarket.StorageDealCheckForAcceptance:    CheckForDealAcceptance,
	storagemarket.StorageDealProposalAccepted:      ValidateDealPublished,
	storagemarket.StorageDealAwaitingPreCommit:     VerifyDealPreCommitted,
	storagemarket.StorageDealSealing:               VerifyDealActivated,
	storagemarket.StorageDealActive:                WaitForDealCompletion,
	storagemarket.StorageDealFailing:               FailDeal,
}

// ClientFinalityStates are the states that terminate deal processing for a deal.
// When a client restarts, it restarts only deals that are not in a finality state.
var ClientFinalityStates = []fsm.StateKey{
	storagemarket.StorageDealSlashed,
	storagemarket.StorageDealExpired,
	storagemarket.StorageDealError,
}
