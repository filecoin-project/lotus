package retrievalmarket

import "fmt"

// DealStatus is the status of a retrieval deal returned by a provider
// in a DealResponse
type DealStatus uint64

const (
	// DealStatusNew is a deal that nothing has happened with yet
	DealStatusNew DealStatus = iota

	// DealStatusUnsealing means the provider is unsealing data
	DealStatusUnsealing

	// DealStatusUnsealed means the provider has finished unsealing data
	DealStatusUnsealed

	// DealStatusWaitForAcceptance means we're waiting to hear back if the provider accepted our deal
	DealStatusWaitForAcceptance

	// DealStatusPaymentChannelCreating is the status set while waiting for the
	// payment channel creation to complete
	DealStatusPaymentChannelCreating

	// DealStatusPaymentChannelAddingFunds is the status when we are waiting for funds
	// to finish being sent to the payment channel
	DealStatusPaymentChannelAddingFunds

	// DealStatusAccepted means a deal has been accepted by a provider
	// and its is ready to proceed with retrieval
	DealStatusAccepted

	// DealStatusFundsNeededUnseal means a deal has been accepted by a provider
	// and payment is needed to unseal the data
	DealStatusFundsNeededUnseal

	// DealStatusFailing indicates something went wrong during a retrieval,
	// and we are cleaning up before terminating with an error
	DealStatusFailing

	// DealStatusRejected indicates the provider rejected a client's deal proposal
	// for some reason
	DealStatusRejected

	// DealStatusFundsNeeded indicates the provider needs a payment voucher to
	// continue processing the deal
	DealStatusFundsNeeded

	// DealStatusSendFunds indicates the client is now going to send funds because we reached the threshold of the last payment
	DealStatusSendFunds

	// DealStatusSendFundsLastPayment indicates the client is now going to send final funds because
	// we reached the threshold of the final payment
	DealStatusSendFundsLastPayment

	// DealStatusOngoing indicates the provider is continuing to process a deal
	DealStatusOngoing

	// DealStatusFundsNeededLastPayment indicates the provider needs a payment voucher
	// in order to complete a deal
	DealStatusFundsNeededLastPayment

	// DealStatusCompleted indicates a deal is complete
	DealStatusCompleted

	// DealStatusDealNotFound indicates an update was received for a deal that could
	// not be identified
	DealStatusDealNotFound

	// DealStatusErrored indicates a deal has terminated in an error
	DealStatusErrored

	// DealStatusBlocksComplete indicates that all blocks have been processed for the piece
	DealStatusBlocksComplete

	// DealStatusFinalizing means the last payment has been received and
	// we are just confirming the deal is complete
	DealStatusFinalizing

	// DealStatusCompleting is just an inbetween state to perform final cleanup of
	// complete deals
	DealStatusCompleting

	// DealStatusCheckComplete is used for when the provided completes without a last payment
	// requested cycle, to verify we have received all blocks
	DealStatusCheckComplete

	// DealStatusCheckFunds means we are looking at the state of funding for the channel to determine
	// if more money is incoming
	DealStatusCheckFunds

	// DealStatusInsufficientFunds indicates we have depleted funds for the retrieval payment channel
	// - we can resume after funds are added
	DealStatusInsufficientFunds

	// DealStatusPaymentChannelAllocatingLane is the status when we are making a lane for this channel
	DealStatusPaymentChannelAllocatingLane

	// DealStatusCancelling means we are cancelling an inprogress deal
	DealStatusCancelling

	// DealStatusCancelled means a deal has been cancelled
	DealStatusCancelled

	// DealStatusRetryLegacy means we're attempting the deal proposal for a second time using the legacy datatype
	DealStatusRetryLegacy

	// DealStatusWaitForAcceptanceLegacy means we're waiting to hear the results on the legacy protocol
	DealStatusWaitForAcceptanceLegacy

	// DealStatusClientWaitingForLastBlocks means that the provider has told
	// the client that all blocks were sent for the deal, and the client is
	// waiting for the last blocks to arrive. This should only happen when
	// the deal price per byte is zero (if it's not zero the provider asks
	// for final payment after sending the last blocks).
	DealStatusClientWaitingForLastBlocks

	// DealStatusPaymentChannelAddingInitialFunds means that a payment channel
	// exists from an earlier deal between client and provider, but we need
	// to add funds to the channel for this particular deal
	DealStatusPaymentChannelAddingInitialFunds

	// DealStatusErroring means that there was an error and we need to
	// do some cleanup before moving to the error state
	DealStatusErroring

	// DealStatusRejecting means that the deal was rejected and we need to do
	// some cleanup before moving to the rejected state
	DealStatusRejecting

	// DealStatusDealNotFoundCleanup means that the deal was not found and we
	// need to do some cleanup before moving to the not found state
	DealStatusDealNotFoundCleanup

	// DealStatusFinalizingBlockstore means that all blocks have been received,
	// and the blockstore is being finalized
	DealStatusFinalizingBlockstore
)

// DealStatuses maps deal status to a human readable representation
var DealStatuses = map[DealStatus]string{
	DealStatusNew:                              "DealStatusNew",
	DealStatusUnsealing:                        "DealStatusUnsealing",
	DealStatusUnsealed:                         "DealStatusUnsealed",
	DealStatusWaitForAcceptance:                "DealStatusWaitForAcceptance",
	DealStatusPaymentChannelCreating:           "DealStatusPaymentChannelCreating",
	DealStatusPaymentChannelAddingFunds:        "DealStatusPaymentChannelAddingFunds",
	DealStatusAccepted:                         "DealStatusAccepted",
	DealStatusFundsNeededUnseal:                "DealStatusFundsNeededUnseal",
	DealStatusFailing:                          "DealStatusFailing",
	DealStatusRejected:                         "DealStatusRejected",
	DealStatusFundsNeeded:                      "DealStatusFundsNeeded",
	DealStatusSendFunds:                        "DealStatusSendFunds",
	DealStatusSendFundsLastPayment:             "DealStatusSendFundsLastPayment",
	DealStatusOngoing:                          "DealStatusOngoing",
	DealStatusFundsNeededLastPayment:           "DealStatusFundsNeededLastPayment",
	DealStatusCompleted:                        "DealStatusCompleted",
	DealStatusDealNotFound:                     "DealStatusDealNotFound",
	DealStatusErrored:                          "DealStatusErrored",
	DealStatusBlocksComplete:                   "DealStatusBlocksComplete",
	DealStatusFinalizing:                       "DealStatusFinalizing",
	DealStatusCompleting:                       "DealStatusCompleting",
	DealStatusCheckComplete:                    "DealStatusCheckComplete",
	DealStatusCheckFunds:                       "DealStatusCheckFunds",
	DealStatusInsufficientFunds:                "DealStatusInsufficientFunds",
	DealStatusPaymentChannelAllocatingLane:     "DealStatusPaymentChannelAllocatingLane",
	DealStatusCancelling:                       "DealStatusCancelling",
	DealStatusCancelled:                        "DealStatusCancelled",
	DealStatusRetryLegacy:                      "DealStatusRetryLegacy",
	DealStatusWaitForAcceptanceLegacy:          "DealStatusWaitForAcceptanceLegacy",
	DealStatusClientWaitingForLastBlocks:       "DealStatusWaitingForLastBlocks",
	DealStatusPaymentChannelAddingInitialFunds: "DealStatusPaymentChannelAddingInitialFunds",
	DealStatusErroring:                         "DealStatusErroring",
	DealStatusRejecting:                        "DealStatusRejecting",
	DealStatusDealNotFoundCleanup:              "DealStatusDealNotFoundCleanup",
	DealStatusFinalizingBlockstore:             "DealStatusFinalizingBlockstore",
}

func (s DealStatus) String() string {
	str, ok := DealStatuses[s]
	if ok {
		return str
	}
	return fmt.Sprintf("DealStatusUnknown - %d", s)
}
