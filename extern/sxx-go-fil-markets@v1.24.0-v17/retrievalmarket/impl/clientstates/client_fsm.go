package clientstates

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

func recordReceived(deal *rm.ClientDealState, totalReceived uint64) error {
	deal.TotalReceived = totalReceived
	return nil
}

var paymentChannelCreationStates = []fsm.StateKey{
	rm.DealStatusWaitForAcceptance,
	rm.DealStatusWaitForAcceptanceLegacy,
	rm.DealStatusAccepted,
	rm.DealStatusPaymentChannelCreating,
	rm.DealStatusPaymentChannelAddingInitialFunds,
	rm.DealStatusPaymentChannelAllocatingLane,
}

// ClientEvents are the events that can happen in a retrieval client
var ClientEvents = fsm.Events{
	fsm.Event(rm.ClientEventOpen).
		From(rm.DealStatusNew).ToNoChange(),

	// ProposeDeal handler events
	fsm.Event(rm.ClientEventWriteDealProposalErrored).
		FromAny().To(rm.DealStatusErroring).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("proposing deal: %w", err).Error()
			return nil
		}),
	fsm.Event(rm.ClientEventDealProposed).
		From(rm.DealStatusNew).To(rm.DealStatusWaitForAcceptance).
		From(rm.DealStatusRetryLegacy).To(rm.DealStatusWaitForAcceptanceLegacy).
		From(rm.DealStatusCancelling).ToJustRecord().
		Action(func(deal *rm.ClientDealState, channelID datatransfer.ChannelID) error {
			deal.ChannelID = &channelID
			deal.Message = ""
			return nil
		}),

	// Initial deal acceptance events
	fsm.Event(rm.ClientEventDealRejected).
		From(rm.DealStatusWaitForAcceptance).To(rm.DealStatusRetryLegacy).
		From(rm.DealStatusWaitForAcceptanceLegacy).To(rm.DealStatusRejecting).
		Action(func(deal *rm.ClientDealState, message string) error {
			deal.Message = fmt.Sprintf("deal rejected: %s", message)
			deal.LegacyProtocol = true
			return nil
		}),
	fsm.Event(rm.ClientEventDealNotFound).
		FromMany(rm.DealStatusWaitForAcceptance, rm.DealStatusWaitForAcceptanceLegacy).To(rm.DealStatusDealNotFoundCleanup).
		Action(func(deal *rm.ClientDealState, message string) error {
			deal.Message = fmt.Sprintf("deal not found: %s", message)
			return nil
		}),
	fsm.Event(rm.ClientEventDealAccepted).
		FromMany(rm.DealStatusWaitForAcceptance, rm.DealStatusWaitForAcceptanceLegacy).To(rm.DealStatusAccepted),
	fsm.Event(rm.ClientEventUnknownResponseReceived).
		FromAny().To(rm.DealStatusFailing).
		Action(func(deal *rm.ClientDealState, status rm.DealStatus) error {
			deal.Message = fmt.Sprintf("Unexpected deal response status: %s", rm.DealStatuses[status])
			return nil
		}),

	// Payment channel setup
	fsm.Event(rm.ClientEventPaymentChannelErrored).
		FromMany(rm.DealStatusAccepted, rm.DealStatusPaymentChannelCreating, rm.DealStatusPaymentChannelAddingFunds).To(rm.DealStatusFailing).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("error from payment channel: %w", err).Error()
			return nil
		}),

	// Price of deal is zero so skip creating a payment channel
	fsm.Event(rm.ClientEventPaymentChannelSkip).
		From(rm.DealStatusAccepted).To(rm.DealStatusOngoing),

	fsm.Event(rm.ClientEventPaymentChannelCreateInitiated).
		From(rm.DealStatusAccepted).To(rm.DealStatusPaymentChannelCreating).
		Action(func(deal *rm.ClientDealState, msgCID cid.Cid) error {
			deal.WaitMsgCID = &msgCID
			return nil
		}),

	// Client is adding funds to payment channel
	fsm.Event(rm.ClientEventPaymentChannelAddingFunds).
		// If the deal has just been accepted, we are adding the initial funds
		// to the payment channel
		FromMany(rm.DealStatusAccepted).To(rm.DealStatusPaymentChannelAddingInitialFunds).
		// If the deal was already ongoing, and ran out of funds, we are
		// topping up funds in the payment channel
		FromMany(rm.DealStatusCheckFunds).To(rm.DealStatusPaymentChannelAddingFunds).
		Action(func(deal *rm.ClientDealState, msgCID cid.Cid, payCh address.Address) error {
			deal.WaitMsgCID = &msgCID
			if deal.PaymentInfo == nil {
				deal.PaymentInfo = &rm.PaymentInfo{
					PayCh: payCh,
				}
			}
			return nil
		}),

	// The payment channel add funds message has landed on chain
	fsm.Event(rm.ClientEventPaymentChannelReady).
		// If the payment channel between client and provider was being created
		// for the first time, or if the payment channel had already been
		// created for an earlier deal but the initial funding for this deal
		// was being added, then we still need to allocate a payment channel
		// lane
		FromMany(rm.DealStatusPaymentChannelCreating, rm.DealStatusPaymentChannelAddingInitialFunds, rm.DealStatusAccepted).To(rm.DealStatusPaymentChannelAllocatingLane).
		// If the payment channel ran out of funds and needed to be topped up,
		// then the payment channel lane already exists so just move straight
		// to the ongoing state
		From(rm.DealStatusPaymentChannelAddingFunds).To(rm.DealStatusOngoing).
		From(rm.DealStatusCheckFunds).To(rm.DealStatusOngoing).
		Action(func(deal *rm.ClientDealState, payCh address.Address) error {
			if deal.PaymentInfo == nil {
				deal.PaymentInfo = &rm.PaymentInfo{
					PayCh: payCh,
				}
			}
			deal.WaitMsgCID = nil
			// remove any insufficient funds message
			deal.Message = ""
			return nil
		}),

	fsm.Event(rm.ClientEventAllocateLaneErrored).
		FromMany(rm.DealStatusPaymentChannelAllocatingLane).
		To(rm.DealStatusFailing).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("allocating payment lane: %w", err).Error()
			return nil
		}),

	fsm.Event(rm.ClientEventLaneAllocated).
		From(rm.DealStatusPaymentChannelAllocatingLane).To(rm.DealStatusOngoing).
		Action(func(deal *rm.ClientDealState, lane uint64) error {
			deal.PaymentInfo.Lane = lane
			return nil
		}),

	// Transfer Channel Errors
	fsm.Event(rm.ClientEventDataTransferError).
		FromAny().To(rm.DealStatusErroring).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = fmt.Sprintf("error generated by data transfer: %s", err.Error())
			return nil
		}),

	// Receiving requests for payment
	fsm.Event(rm.ClientEventLastPaymentRequested).
		FromMany(
			rm.DealStatusOngoing,
			rm.DealStatusFundsNeededLastPayment,
			rm.DealStatusFundsNeeded).To(rm.DealStatusFundsNeededLastPayment).
		From(rm.DealStatusSendFunds).To(rm.DealStatusOngoing).
		From(rm.DealStatusCheckComplete).ToNoChange().
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusSendFundsLastPayment).
		FromMany(
			paymentChannelCreationStates...).ToJustRecord().
		Action(func(deal *rm.ClientDealState, paymentOwed abi.TokenAmount) error {
			deal.PaymentRequested = big.Add(deal.PaymentRequested, paymentOwed)
			deal.LastPaymentRequested = true
			return nil
		}),
	fsm.Event(rm.ClientEventPaymentRequested).
		FromMany(
			rm.DealStatusOngoing,
			rm.DealStatusBlocksComplete,
			rm.DealStatusFundsNeeded,
			rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusFundsNeeded).
		From(rm.DealStatusSendFunds).To(rm.DealStatusOngoing).
		From(rm.DealStatusCheckComplete).ToNoChange().
		FromMany(
			paymentChannelCreationStates...).ToJustRecord().
		Action(func(deal *rm.ClientDealState, paymentOwed abi.TokenAmount) error {
			deal.PaymentRequested = big.Add(deal.PaymentRequested, paymentOwed)
			return nil
		}),

	fsm.Event(rm.ClientEventUnsealPaymentRequested).
		FromMany(rm.DealStatusWaitForAcceptance, rm.DealStatusWaitForAcceptanceLegacy).To(rm.DealStatusAccepted).
		Action(func(deal *rm.ClientDealState, paymentOwed abi.TokenAmount) error {
			deal.PaymentRequested = big.Add(deal.PaymentRequested, paymentOwed)
			return nil
		}),

	// Receiving data
	fsm.Event(rm.ClientEventAllBlocksReceived).
		FromMany(
			rm.DealStatusOngoing,
			rm.DealStatusBlocksComplete,
		).To(rm.DealStatusBlocksComplete).
		FromMany(paymentChannelCreationStates...).ToJustRecord().
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusOngoing).
		From(rm.DealStatusFundsNeeded).ToNoChange().
		From(rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusSendFundsLastPayment).
		From(rm.DealStatusClientWaitingForLastBlocks).To(rm.DealStatusFinalizingBlockstore).
		From(rm.DealStatusCheckComplete).To(rm.DealStatusFinalizingBlockstore).
		Action(func(deal *rm.ClientDealState) error {
			deal.AllBlocksReceived = true
			return nil
		}),
	fsm.Event(rm.ClientEventBlocksReceived).
		FromMany(rm.DealStatusOngoing,
			rm.DealStatusFundsNeeded,
			rm.DealStatusFundsNeededLastPayment,
			rm.DealStatusCheckComplete,
			rm.DealStatusClientWaitingForLastBlocks).ToNoChange().
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusOngoing).
		FromMany(paymentChannelCreationStates...).ToJustRecord().
		Action(recordReceived),

	fsm.Event(rm.ClientEventSendFunds).
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusOngoing).
		From(rm.DealStatusFundsNeeded).To(rm.DealStatusSendFunds).
		From(rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusSendFundsLastPayment),

	// Sending Payments
	fsm.Event(rm.ClientEventFundsExpended).
		FromMany(rm.DealStatusCheckFunds).To(rm.DealStatusInsufficientFunds).
		Action(func(deal *rm.ClientDealState, shortfall abi.TokenAmount) error {
			deal.Message = fmt.Sprintf("not enough current or pending funds in payment channel, shortfall of %s", shortfall.String())
			return nil
		}),
	fsm.Event(rm.ClientEventBadPaymentRequested).
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusFailing).
		Action(func(deal *rm.ClientDealState, message string) error {
			deal.Message = message
			return nil
		}),
	fsm.Event(rm.ClientEventCreateVoucherFailed).
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusFailing).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("creating payment voucher: %w", err).Error()
			return nil
		}),
	fsm.Event(rm.ClientEventVoucherShortfall).
		FromMany(rm.DealStatusSendFunds, rm.DealStatusSendFundsLastPayment).To(rm.DealStatusCheckFunds).
		Action(func(deal *rm.ClientDealState, shortfall abi.TokenAmount) error {
			return nil
		}),

	fsm.Event(rm.ClientEventWriteDealPaymentErrored).
		FromAny().To(rm.DealStatusErroring).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("writing deal payment: %w", err).Error()
			return nil
		}),

	// Payment was requested, but there was not actually any payment due, so
	// no payment voucher was actually sent
	fsm.Event(rm.ClientEventPaymentNotSent).
		From(rm.DealStatusOngoing).ToJustRecord().
		From(rm.DealStatusSendFunds).To(rm.DealStatusOngoing).
		From(rm.DealStatusSendFundsLastPayment).To(rm.DealStatusFinalizing),

	fsm.Event(rm.ClientEventPaymentSent).
		From(rm.DealStatusOngoing).ToJustRecord().
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusCheckComplete).
		From(rm.DealStatusCheckComplete).ToNoChange().
		FromMany(
			rm.DealStatusFundsNeeded,
			rm.DealStatusFundsNeededLastPayment,
			rm.DealStatusSendFunds).To(rm.DealStatusOngoing).
		From(rm.DealStatusSendFundsLastPayment).To(rm.DealStatusFinalizing).
		Action(func(deal *rm.ClientDealState, voucherAmt abi.TokenAmount) error {
			// Reduce the payment requested by the amount of funds sent.
			// Note that it may not be reduced to zero, if a new payment
			// request came in while this one was being processed.
			sentAmt := big.Sub(voucherAmt, deal.FundsSpent)
			deal.PaymentRequested = big.Sub(deal.PaymentRequested, sentAmt)

			// Update the total funds sent to the provider
			deal.FundsSpent = voucherAmt

			// If the unseal price hasn't yet been met, set the unseal funds
			// paid to the amount sent to the provider
			if deal.UnsealPrice.GreaterThanEqual(deal.FundsSpent) {
				deal.UnsealFundsPaid = deal.FundsSpent
				return nil
			}
			// The unseal funds have been fully paid
			deal.UnsealFundsPaid = deal.UnsealPrice

			// If the price per byte is zero, no further accounting needed
			if deal.PricePerByte.IsZero() {
				return nil
			}

			// Calculate the amount spent on transferring data, and update the
			// bytes paid for accordingly
			paidSoFarForTransfer := big.Sub(deal.FundsSpent, deal.UnsealFundsPaid)
			deal.BytesPaidFor = big.Div(paidSoFarForTransfer, deal.PricePerByte).Uint64()

			// If the number of bytes paid for is above the current interval,
			// increase the interval
			if deal.BytesPaidFor >= deal.CurrentInterval {
				deal.CurrentInterval = deal.NextInterval()
			}

			return nil
		}),

	// completing deals
	fsm.Event(rm.ClientEventComplete).
		FromMany(
			rm.DealStatusSendFunds,
			rm.DealStatusSendFundsLastPayment,
			rm.DealStatusFundsNeeded,
			rm.DealStatusFundsNeededLastPayment).To(rm.DealStatusCheckComplete).
		From(rm.DealStatusOngoing).To(rm.DealStatusCheckComplete).
		From(rm.DealStatusBlocksComplete).To(rm.DealStatusCheckComplete).
		From(rm.DealStatusFinalizing).To(rm.DealStatusFinalizingBlockstore),
	fsm.Event(rm.ClientEventCompleteVerified).
		From(rm.DealStatusCheckComplete).To(rm.DealStatusFinalizingBlockstore),
	fsm.Event(rm.ClientEventEarlyTermination).
		From(rm.DealStatusCheckComplete).To(rm.DealStatusErroring).
		Action(func(deal *rm.ClientDealState) error {
			deal.Message = "Provider sent complete status without sending all data"
			return nil
		}),

	// the provider indicated that all blocks have been sent, so the client
	// should wait for the last blocks to arrive (only needed when price
	// per byte is zero)
	fsm.Event(rm.ClientEventWaitForLastBlocks).
		From(rm.DealStatusCheckComplete).To(rm.DealStatusClientWaitingForLastBlocks).
		FromMany(rm.DealStatusFinalizingBlockstore, rm.DealStatusCompleted).ToJustRecord(),

	// Once all blocks have been received and the blockstore has been finalized,
	// move to the complete state
	fsm.Event(rm.ClientEventBlockstoreFinalized).
		From(rm.DealStatusFinalizingBlockstore).To(rm.DealStatusCompleted).
		From(rm.DealStatusErroring).To(rm.DealStatusErrored).
		From(rm.DealStatusRejecting).To(rm.DealStatusRejected).
		From(rm.DealStatusDealNotFoundCleanup).To(rm.DealStatusDealNotFound),

	// An error occurred when finalizing the blockstore
	fsm.Event(rm.ClientEventFinalizeBlockstoreErrored).
		From(rm.DealStatusFinalizingBlockstore).To(rm.DealStatusErrored).
		Action(func(deal *rm.ClientDealState, err error) error {
			deal.Message = xerrors.Errorf("finalizing blockstore: %w", err).Error()
			return nil
		}),

	// after cancelling a deal is complete
	fsm.Event(rm.ClientEventCancelComplete).
		From(rm.DealStatusFailing).To(rm.DealStatusErrored).
		From(rm.DealStatusCancelling).To(rm.DealStatusCancelled),

	// receiving a cancel indicating most likely that the provider experienced something wrong on their
	// end, unless we are already failing or cancelling
	fsm.Event(rm.ClientEventProviderCancelled).
		From(rm.DealStatusFailing).ToJustRecord().
		From(rm.DealStatusCancelling).ToJustRecord().
		FromAny().To(rm.DealStatusCancelling).Action(
		func(deal *rm.ClientDealState) error {
			if deal.Status != rm.DealStatusFailing && deal.Status != rm.DealStatusCancelling {
				deal.Message = "Provider cancelled retrieval"
			}
			return nil
		},
	),

	// user manually cancels retrieval
	fsm.Event(rm.ClientEventCancel).FromAny().To(rm.DealStatusCancelling).Action(func(deal *rm.ClientDealState) error {
		deal.Message = "Client cancelled retrieval"
		return nil
	}),

	// payment channel receives more money, we believe there may be reason to recheck the funds for this channel
	fsm.Event(rm.ClientEventRecheckFunds).From(rm.DealStatusInsufficientFunds).To(rm.DealStatusCheckFunds),
}

// ClientFinalityStates are terminal states after which no further events are received
var ClientFinalityStates = []fsm.StateKey{
	rm.DealStatusErrored,
	rm.DealStatusCompleted,
	rm.DealStatusCancelled,
	rm.DealStatusRejected,
	rm.DealStatusDealNotFound,
}

func IsFinalityState(st fsm.StateKey) bool {
	for _, state := range ClientFinalityStates {
		if st == state {
			return true
		}
	}
	return false
}

// ClientStateEntryFuncs are the handlers for different states in a retrieval client
var ClientStateEntryFuncs = fsm.StateEntryFuncs{
	rm.DealStatusNew:                              ProposeDeal,
	rm.DealStatusRetryLegacy:                      ProposeDeal,
	rm.DealStatusAccepted:                         SetupPaymentChannelStart,
	rm.DealStatusPaymentChannelCreating:           WaitPaymentChannelReady,
	rm.DealStatusPaymentChannelAddingInitialFunds: WaitPaymentChannelReady,
	rm.DealStatusPaymentChannelAllocatingLane:     AllocateLane,
	rm.DealStatusOngoing:                          Ongoing,
	rm.DealStatusFundsNeeded:                      ProcessPaymentRequested,
	rm.DealStatusFundsNeededLastPayment:           ProcessPaymentRequested,
	rm.DealStatusSendFunds:                        SendFunds,
	rm.DealStatusSendFundsLastPayment:             SendFunds,
	rm.DealStatusCheckFunds:                       CheckFunds,
	rm.DealStatusPaymentChannelAddingFunds:        WaitPaymentChannelReady,
	rm.DealStatusFailing:                          CancelDeal,
	rm.DealStatusCancelling:                       CancelDeal,
	rm.DealStatusCheckComplete:                    CheckComplete,
	rm.DealStatusFinalizingBlockstore:             FinalizeBlockstore,
	rm.DealStatusErroring:                         FailsafeFinalizeBlockstore,
	rm.DealStatusRejecting:                        FailsafeFinalizeBlockstore,
	rm.DealStatusDealNotFoundCleanup:              FailsafeFinalizeBlockstore,
}
