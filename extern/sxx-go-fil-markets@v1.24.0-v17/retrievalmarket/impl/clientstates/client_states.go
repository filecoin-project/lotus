package clientstates

import (
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("markets-rtvl")

// ClientDealEnvironment is a bridge to the environment a client deal is executing in.
// It provides access to relevant functionality on the retrieval client
type ClientDealEnvironment interface {
	// Node returns the node interface for this deal
	Node() rm.RetrievalClientNode
	OpenDataTransfer(ctx context.Context, to peer.ID, proposal *rm.DealProposal, legacy bool) (datatransfer.ChannelID, error)
	SendDataTransferVoucher(context.Context, datatransfer.ChannelID, *rm.DealPayment, bool) error
	CloseDataTransfer(context.Context, datatransfer.ChannelID) error
	FinalizeBlockstore(context.Context, rm.DealID) error
}

// ProposeDeal sends the proposal to the other party
func ProposeDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	legacy := deal.Status == rm.DealStatusRetryLegacy
	channelID, err := environment.OpenDataTransfer(ctx.Context(), deal.Sender, &deal.DealProposal, legacy)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealProposalErrored, err)
	}
	return ctx.Trigger(rm.ClientEventDealProposed, channelID)
}

// SetupPaymentChannelStart initiates setting up a payment channel for a deal
func SetupPaymentChannelStart(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// If the total funds required for the deal are zero, skip creating the payment channel
	if deal.TotalFunds.IsZero() {
		return ctx.Trigger(rm.ClientEventPaymentChannelSkip)
	}

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	paych, msgCID, err := environment.Node().GetOrCreatePaymentChannel(ctx.Context(), deal.ClientWallet, deal.MinerWallet, deal.TotalFunds, tok)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}

	if paych == address.Undef {
		return ctx.Trigger(rm.ClientEventPaymentChannelCreateInitiated, msgCID)
	}

	if msgCID == cid.Undef {
		return ctx.Trigger(rm.ClientEventPaymentChannelReady, paych)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, msgCID, paych)
}

// WaitPaymentChannelReady waits for a pending operation on a payment channel -- either creating or depositing funds
func WaitPaymentChannelReady(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	paych, err := environment.Node().WaitForPaymentChannelReady(ctx.Context(), *deal.WaitMsgCID)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelReady, paych)
}

// AllocateLane allocates a lane for this retrieval operation
func AllocateLane(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	lane, err := environment.Node().AllocateLane(ctx.Context(), deal.PaymentInfo.PayCh)
	if err != nil {
		return ctx.Trigger(rm.ClientEventAllocateLaneErrored, err)
	}
	return ctx.Trigger(rm.ClientEventLaneAllocated, lane)
}

// Ongoing just double checks that we may need to move out of the ongoing state cause a payment was previously requested
func Ongoing(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	if deal.PaymentRequested.GreaterThan(big.Zero()) {
		if deal.LastPaymentRequested {
			return ctx.Trigger(rm.ClientEventLastPaymentRequested, big.Zero())
		}
		return ctx.Trigger(rm.ClientEventPaymentRequested, big.Zero())
	}
	return nil
}

// ProcessPaymentRequested processes a request for payment from the provider
func ProcessPaymentRequested(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// If the unseal payment hasn't been made, we need to send funds
	if deal.UnsealPrice.GreaterThan(deal.UnsealFundsPaid) {
		log.Debugf("client: payment needed: unseal price %d > unseal paid %d",
			deal.UnsealPrice, deal.UnsealFundsPaid)
		return ctx.Trigger(rm.ClientEventSendFunds)
	}

	// If all bytes received have been paid for, we don't need to send funds
	if deal.BytesPaidFor >= deal.TotalReceived {
		log.Debugf("client: no payment needed: bytes paid for %d >= bytes received %d",
			deal.BytesPaidFor, deal.TotalReceived)
		return nil
	}

	// Not all bytes received have been paid for

	// If all blocks have been received we need to send a final payment
	if deal.AllBlocksReceived {
		log.Debugf("client: payment needed: all blocks received, bytes paid for %d < bytes received %d",
			deal.BytesPaidFor, deal.TotalReceived)
		return ctx.Trigger(rm.ClientEventSendFunds)
	}

	// Payments are made in intervals, as bytes are received from the provider.
	// If the number of bytes received is at or above the size of the current
	// interval, we need to send a payment.
	if deal.TotalReceived >= deal.CurrentInterval {
		log.Debugf("client: payment needed: bytes received %d >= interval %d, bytes paid for %d < bytes received %d",
			deal.TotalReceived, deal.CurrentInterval, deal.BytesPaidFor, deal.TotalReceived)
		return ctx.Trigger(rm.ClientEventSendFunds)
	}

	log.Debugf("client: no payment needed: received %d < interval %d (paid for %d)",
		deal.TotalReceived, deal.CurrentInterval, deal.BytesPaidFor)
	return nil
}

// SendFunds sends the next amount requested by the provider
func SendFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	totalBytesToPayFor := deal.TotalReceived

	// If unsealing has been paid for, and not all blocks have been received,
	// and the number of bytes received is less than the number required
	// for the current payment interval, no need to send a payment
	if deal.UnsealFundsPaid.GreaterThanEqual(deal.UnsealPrice) &&
		!deal.AllBlocksReceived &&
		totalBytesToPayFor < deal.CurrentInterval {

		log.Debugf("client: ignoring payment request for %d: total bytes to pay for %d < interval %d",
			deal.PaymentRequested, totalBytesToPayFor, deal.CurrentInterval)
		return ctx.Trigger(rm.ClientEventPaymentNotSent)
	}

	tok, _, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(rm.ClientEventCreateVoucherFailed, err)
	}

	// Calculate the payment amount due for data received
	transferPrice := big.Mul(abi.NewTokenAmount(int64(totalBytesToPayFor)), deal.PricePerByte)
	// Calculate the total amount including the unsealing cost
	totalPrice := big.Add(transferPrice, deal.UnsealPrice)

	// If we've already sent at or above the amount due, no need to send funds
	if totalPrice.LessThanEqual(deal.FundsSpent) {
		log.Debugf("client: not sending voucher: funds spent %d >= total price %d: transfer price %d + unseal price %d (payment requested %d)",
			deal.FundsSpent, totalPrice, transferPrice, deal.UnsealPrice, deal.PaymentRequested)
		return ctx.Trigger(rm.ClientEventPaymentNotSent)
	}

	log.Debugf("client: sending voucher for %d = transfer price %d + unseal price %d (payment requested %d)",
		totalPrice, transferPrice, deal.UnsealPrice, deal.PaymentRequested)

	// Create a payment voucher
	voucher, err := environment.Node().CreatePaymentVoucher(ctx.Context(), deal.PaymentInfo.PayCh, totalPrice, deal.PaymentInfo.Lane, tok)
	if err != nil {
		shortfallErr, ok := err.(rm.ShortfallError)
		if ok {
			// There were not enough funds in the payment channel to create a
			// voucher of this amount, so the client needs to add more funds to
			// the payment channel
			log.Debugf("client: voucher shortfall of %d when creating voucher for %d",
				shortfallErr.Shortfall(), totalPrice)
			return ctx.Trigger(rm.ClientEventVoucherShortfall, shortfallErr.Shortfall())
		}
		return ctx.Trigger(rm.ClientEventCreateVoucherFailed, err)
	}

	// Send the payment voucher
	err = environment.SendDataTransferVoucher(ctx.Context(), *deal.ChannelID, &rm.DealPayment{
		ID:             deal.DealProposal.ID,
		PaymentChannel: deal.PaymentInfo.PayCh,
		PaymentVoucher: voucher,
	}, deal.LegacyProtocol)
	if err != nil {
		return ctx.Trigger(rm.ClientEventWriteDealPaymentErrored, err)
	}

	return ctx.Trigger(rm.ClientEventPaymentSent, totalPrice)
}

// CheckFunds examines current available funds in a payment channel after a voucher shortfall to determine
// a course of action -- whether it's a good time to try again, wait for pending operations, or
// we've truly expended all funds and we need to wait for a manual readd
func CheckFunds(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// if we already have an outstanding operation, let's wait for that to complete
	if deal.WaitMsgCID != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, *deal.WaitMsgCID, deal.PaymentInfo.PayCh)
	}
	availableFunds, err := environment.Node().CheckAvailableFunds(ctx.Context(), deal.PaymentInfo.PayCh)
	if err != nil {
		return ctx.Trigger(rm.ClientEventPaymentChannelErrored, err)
	}
	unredeemedFunds := big.Sub(availableFunds.ConfirmedAmt, availableFunds.VoucherReedeemedAmt)
	shortfall := big.Sub(deal.PaymentRequested, unredeemedFunds)
	if shortfall.LessThanEqual(big.Zero()) {
		return ctx.Trigger(rm.ClientEventPaymentChannelReady, deal.PaymentInfo.PayCh)
	}
	totalInFlight := big.Add(availableFunds.PendingAmt, availableFunds.QueuedAmt)
	if totalInFlight.LessThan(shortfall) || availableFunds.PendingWaitSentinel == nil {
		finalShortfall := big.Sub(shortfall, totalInFlight)
		return ctx.Trigger(rm.ClientEventFundsExpended, finalShortfall)
	}
	return ctx.Trigger(rm.ClientEventPaymentChannelAddingFunds, *availableFunds.PendingWaitSentinel, deal.PaymentInfo.PayCh)
}

// CancelDeal clears a deal that went wrong for an unknown reason
func CancelDeal(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// Attempt to finalize the blockstore. If it fails just log an error as
	// we want to make sure we end up in the cancelled state (not an error
	// state)
	if err := environment.FinalizeBlockstore(ctx.Context(), deal.ID); err != nil {
		log.Errorf("failed to finalize blockstore for deal %s: %s", deal.ID, err)
	}

	// If the data transfer has started, cancel it
	if deal.ChannelID != nil {
		// Read next response (or fail)
		err := environment.CloseDataTransfer(ctx.Context(), *deal.ChannelID)
		if err != nil {
			return ctx.Trigger(rm.ClientEventDataTransferError, err)
		}
	}

	return ctx.Trigger(rm.ClientEventCancelComplete)
}

// CheckComplete verifies that a provider that completed without a last payment requested did in fact send us all the data
func CheckComplete(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// This function is called when the provider tells the client that it has
	// sent all the blocks, so check if all blocks have been received.
	if deal.AllBlocksReceived {
		return ctx.Trigger(rm.ClientEventCompleteVerified)
	}

	// If the deal price per byte is zero, wait for the last blocks to
	// arrive
	if deal.PricePerByte.IsZero() {
		return ctx.Trigger(rm.ClientEventWaitForLastBlocks)
	}

	// If the deal price per byte is non-zero, the provider should only
	// have sent the complete message after receiving the last payment
	// from the client, which should happen after all blocks have been
	// received. So if they haven't been received the provider is trying
	// to terminate the deal early.
	return ctx.Trigger(rm.ClientEventEarlyTermination)
}

// FinalizeBlockstore is called once all blocks have been received and the
// blockstore needs to be finalized before completing the deal
func FinalizeBlockstore(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	if err := environment.FinalizeBlockstore(ctx.Context(), deal.ID); err != nil {
		return ctx.Trigger(rm.ClientEventFinalizeBlockstoreErrored, err)
	}
	return ctx.Trigger(rm.ClientEventBlockstoreFinalized)
}

// FailsafeFinalizeBlockstore is called when there is a termination state
// because of some irregularity (eg deal not found).
// It attempts to clean up the blockstore, but even if there's an error it
// always fires a blockstore finalized event so that we still end up in the
// appropriate termination state.
func FailsafeFinalizeBlockstore(ctx fsm.Context, environment ClientDealEnvironment, deal rm.ClientDealState) error {
	// Attempt to finalize the blockstore. If it fails just log an error as
	// we want to make sure we end up in a specific termination state (not
	// necessarily the error state)
	if err := environment.FinalizeBlockstore(ctx.Context(), deal.ID); err != nil {
		log.Errorf("failed to finalize blockstore for deal %s: %s", deal.ID, err)
	}
	return ctx.Trigger(rm.ClientEventBlockstoreFinalized)
}
