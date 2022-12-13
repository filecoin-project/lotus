package retrievalmarket

import "fmt"

// ClientEvent is an event that occurs in a deal lifecycle on the client
type ClientEvent uint64

const (
	// ClientEventOpen indicates a deal was initiated
	ClientEventOpen ClientEvent = iota

	// ClientEventWriteDealProposalErrored means a network error writing a deal proposal
	ClientEventWriteDealProposalErrored

	// ClientEventDealProposed means a deal was successfully sent to a miner
	ClientEventDealProposed

	// ClientEventDealRejected means a deal was rejected by the provider
	ClientEventDealRejected

	// ClientEventDealNotFound means a provider could not find a piece for a deal
	ClientEventDealNotFound

	// ClientEventDealAccepted means a provider accepted a deal
	ClientEventDealAccepted

	// ClientEventProviderCancelled means a provider has sent a message to cancel a deal
	ClientEventProviderCancelled

	// ClientEventUnknownResponseReceived means a client received a response it doesn't
	// understand from the provider
	ClientEventUnknownResponseReceived

	// ClientEventPaymentChannelErrored means there was a failure creating a payment channel
	ClientEventPaymentChannelErrored

	// ClientEventAllocateLaneErrored means there was a failure creating a lane in a payment channel
	ClientEventAllocateLaneErrored

	// ClientEventPaymentChannelCreateInitiated means we are waiting for a message to
	// create a payment channel to appear on chain
	ClientEventPaymentChannelCreateInitiated

	// ClientEventPaymentChannelReady means the newly created payment channel is ready for the
	// deal to resume
	ClientEventPaymentChannelReady

	// ClientEventPaymentChannelAddingFunds mean we are waiting for funds to be
	// added to a payment channel
	ClientEventPaymentChannelAddingFunds

	// ClientEventPaymentChannelAddFundsErrored means that adding funds to the payment channel
	// failed
	ClientEventPaymentChannelAddFundsErrored

	// ClientEventLastPaymentRequested indicates the provider requested a final payment
	ClientEventLastPaymentRequested

	// ClientEventAllBlocksReceived indicates the provider has sent all blocks
	ClientEventAllBlocksReceived

	// ClientEventPaymentRequested indicates the provider requested a payment
	ClientEventPaymentRequested

	// ClientEventUnsealPaymentRequested indicates the provider requested a payment for unsealing the sector
	ClientEventUnsealPaymentRequested

	// ClientEventBlocksReceived indicates the provider has sent blocks
	ClientEventBlocksReceived

	// ClientEventSendFunds emits when we reach the threshold to send the next payment
	ClientEventSendFunds

	// ClientEventFundsExpended indicates a deal has run out of funds in the payment channel
	// forcing the client to add more funds to continue the deal
	ClientEventFundsExpended // when totalFunds is expended

	// ClientEventBadPaymentRequested indicates the provider asked for funds
	// in a way that does not match the terms of the deal
	ClientEventBadPaymentRequested

	// ClientEventCreateVoucherFailed indicates an error happened creating a payment voucher
	ClientEventCreateVoucherFailed

	// ClientEventWriteDealPaymentErrored indicates a network error trying to write a payment
	ClientEventWriteDealPaymentErrored

	// ClientEventPaymentSent indicates a payment was sent to the provider
	ClientEventPaymentSent

	// ClientEventComplete is fired when the provider sends a message
	// indicating that a deal has completed
	ClientEventComplete

	// ClientEventDataTransferError emits when something go wrong at the data transfer level
	ClientEventDataTransferError

	// ClientEventCancelComplete happens when a deal cancellation is transmitted to the provider
	ClientEventCancelComplete

	// ClientEventEarlyTermination indications a provider send a deal complete without sending all data
	ClientEventEarlyTermination

	// ClientEventCompleteVerified means that a provider completed without requesting a final payment but
	// we verified we received all data
	ClientEventCompleteVerified

	// ClientEventLaneAllocated is called when a lane is allocated
	ClientEventLaneAllocated

	// ClientEventVoucherShortfall means we tried to create a voucher but did not have enough funds in channel
	// to create it
	ClientEventVoucherShortfall

	// ClientEventRecheckFunds runs when an external caller indicates there may be new funds in a payment channel
	ClientEventRecheckFunds

	// ClientEventCancel runs when a user cancels a deal
	ClientEventCancel

	// ClientEventWaitForLastBlocks is fired when the provider has told
	// the client that all blocks were sent for the deal, and the client is
	// waiting for the last blocks to arrive
	ClientEventWaitForLastBlocks

	// ClientEventPaymentChannelSkip is fired when the total deal price is zero
	// so there's no need to set up a payment channel
	ClientEventPaymentChannelSkip

	// ClientEventPaymentNotSent indicates that payment was requested, but no
	// payment was actually due, so a voucher was not sent to the provider
	ClientEventPaymentNotSent

	// ClientEventBlockstoreFinalized is fired when the blockstore has been
	// finalized after receiving all blocks
	ClientEventBlockstoreFinalized

	// ClientEventFinalizeBlockstoreErrored is fired when there is an error
	// finalizing the blockstore
	ClientEventFinalizeBlockstoreErrored
)

// ClientEvents is a human readable map of client event name -> event description
var ClientEvents = map[ClientEvent]string{
	ClientEventOpen:                          "ClientEventOpen",
	ClientEventPaymentChannelErrored:         "ClientEventPaymentChannelErrored",
	ClientEventDealProposed:                  "ClientEventDealProposed",
	ClientEventAllocateLaneErrored:           "ClientEventAllocateLaneErrored",
	ClientEventPaymentChannelCreateInitiated: "ClientEventPaymentChannelCreateInitiated",
	ClientEventPaymentChannelReady:           "ClientEventPaymentChannelReady",
	ClientEventPaymentChannelAddingFunds:     "ClientEventPaymentChannelAddingFunds",
	ClientEventPaymentChannelAddFundsErrored: "ClientEventPaymentChannelAddFundsErrored",
	ClientEventWriteDealProposalErrored:      "ClientEventWriteDealProposalErrored",
	ClientEventDealRejected:                  "ClientEventDealRejected",
	ClientEventDealNotFound:                  "ClientEventDealNotFound",
	ClientEventDealAccepted:                  "ClientEventDealAccepted",
	ClientEventProviderCancelled:             "ClientEventProviderCancelled",
	ClientEventUnknownResponseReceived:       "ClientEventUnknownResponseReceived",
	ClientEventLastPaymentRequested:          "ClientEventLastPaymentRequested",
	ClientEventAllBlocksReceived:             "ClientEventAllBlocksReceived",
	ClientEventPaymentRequested:              "ClientEventPaymentRequested",
	ClientEventUnsealPaymentRequested:        "ClientEventUnsealPaymentRequested",
	ClientEventBlocksReceived:                "ClientEventBlocksReceived",
	ClientEventSendFunds:                     "ClientEventSendFunds",
	ClientEventFundsExpended:                 "ClientEventFundsExpended",
	ClientEventBadPaymentRequested:           "ClientEventBadPaymentRequested",
	ClientEventCreateVoucherFailed:           "ClientEventCreateVoucherFailed",
	ClientEventWriteDealPaymentErrored:       "ClientEventWriteDealPaymentErrored",
	ClientEventPaymentSent:                   "ClientEventPaymentSent",
	ClientEventDataTransferError:             "ClientEventDataTransferError",
	ClientEventComplete:                      "ClientEventComplete",
	ClientEventCancelComplete:                "ClientEventCancelComplete",
	ClientEventEarlyTermination:              "ClientEventEarlyTermination",
	ClientEventCompleteVerified:              "ClientEventCompleteVerified",
	ClientEventLaneAllocated:                 "ClientEventLaneAllocated",
	ClientEventVoucherShortfall:              "ClientEventVoucherShortfall",
	ClientEventRecheckFunds:                  "ClientEventRecheckFunds",
	ClientEventCancel:                        "ClientEventCancel",
	ClientEventWaitForLastBlocks:             "ClientEventWaitForLastBlocks",
	ClientEventPaymentChannelSkip:            "ClientEventPaymentChannelSkip",
	ClientEventPaymentNotSent:                "ClientEventPaymentNotSent",
	ClientEventBlockstoreFinalized:           "ClientEventBlockstoreFinalized",
	ClientEventFinalizeBlockstoreErrored:     "ClientEventFinalizeBlockstoreErrored",
}

func (e ClientEvent) String() string {
	s, ok := ClientEvents[e]
	if ok {
		return s
	}
	return fmt.Sprintf("ClientEventUnknown: %d", e)
}

// ProviderEvent is an event that occurs in a deal lifecycle on the provider
type ProviderEvent uint64

const (
	// ProviderEventOpen indicates a new deal was received from a client
	ProviderEventOpen ProviderEvent = iota

	// ProviderEventDealNotFound happens when the provider cannot find the piece for the
	// deal proposed by the client
	ProviderEventDealNotFound

	// ProviderEventDealRejected happens when a provider rejects a deal proposed
	// by the client
	ProviderEventDealRejected

	// ProviderEventDealAccepted happens when a provider accepts a deal
	ProviderEventDealAccepted

	// ProviderEventBlockSent happens when the provider reads another block
	// in the piece
	ProviderEventBlockSent

	// ProviderEventBlocksCompleted happens when the provider reads the last block
	// in the piece
	ProviderEventBlocksCompleted

	// ProviderEventPaymentRequested happens when a provider asks for payment from
	// a client for blocks sent
	ProviderEventPaymentRequested

	// ProviderEventSaveVoucherFailed happens when an attempt to save a payment
	// voucher fails
	ProviderEventSaveVoucherFailed

	// ProviderEventPartialPaymentReceived happens when a provider receives and processes
	// a payment that is less than what was requested to proceed with the deal
	ProviderEventPartialPaymentReceived

	// ProviderEventPaymentReceived happens when a provider receives a payment
	// and resumes processing a deal
	ProviderEventPaymentReceived

	// ProviderEventComplete indicates a retrieval deal was completed for a client
	ProviderEventComplete

	// ProviderEventUnsealError emits when something wrong occurs while unsealing data
	ProviderEventUnsealError

	// ProviderEventUnsealComplete emits when the unsealing process is done
	ProviderEventUnsealComplete

	// ProviderEventDataTransferError emits when something go wrong at the data transfer level
	ProviderEventDataTransferError

	// ProviderEventCancelComplete happens when a deal cancellation is transmitted to the provider
	ProviderEventCancelComplete

	// ProviderEventCleanupComplete happens when a deal is finished cleaning up and enters a complete state
	ProviderEventCleanupComplete

	// ProviderEventMultiStoreError occurs when an error happens attempting to operate on the multistore
	ProviderEventMultiStoreError

	// ProviderEventClientCancelled happens when the provider gets a cancel message from the client's data transfer
	ProviderEventClientCancelled
)

// ProviderEvents is a human readable map of provider event name -> event description
var ProviderEvents = map[ProviderEvent]string{
	ProviderEventOpen:                   "ProviderEventOpen",
	ProviderEventDealNotFound:           "ProviderEventDealNotFound",
	ProviderEventDealRejected:           "ProviderEventDealRejected",
	ProviderEventDealAccepted:           "ProviderEventDealAccepted",
	ProviderEventBlockSent:              "ProviderEventBlockSent",
	ProviderEventBlocksCompleted:        "ProviderEventBlocksCompleted",
	ProviderEventPaymentRequested:       "ProviderEventPaymentRequested",
	ProviderEventSaveVoucherFailed:      "ProviderEventSaveVoucherFailed",
	ProviderEventPartialPaymentReceived: "ProviderEventPartialPaymentReceived",
	ProviderEventPaymentReceived:        "ProviderEventPaymentReceived",
	ProviderEventComplete:               "ProviderEventComplete",
	ProviderEventUnsealError:            "ProviderEventUnsealError",
	ProviderEventUnsealComplete:         "ProviderEventUnsealComplete",
	ProviderEventDataTransferError:      "ProviderEventDataTransferError",
	ProviderEventCancelComplete:         "ProviderEventCancelComplete",
	ProviderEventCleanupComplete:        "ProviderEventCleanupComplete",
	ProviderEventMultiStoreError:        "ProviderEventMultiStoreError",
	ProviderEventClientCancelled:        "ProviderEventClientCancelled",
}
