// Package dtutils provides event listeners for the client and provider to
// listen for events on the data transfer module and dispatch FSM events based on them
package dtutils

import (
	"fmt"
	"math"

	"github.com/ipfs/go-graphsync/storeutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p/core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	dtgs "github.com/filecoin-project/go-data-transfer/v2/transport/graphsync"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

var log = logging.Logger("retrievalmarket_impl")

// EventReceiver is any thing that can receive FSM events
type EventReceiver interface {
	Send(id interface{}, name fsm.EventName, args ...interface{}) (err error)
}

const noProviderEvent = rm.ProviderEvent(math.MaxUint64)

func providerEvent(event datatransfer.Event, channelState datatransfer.ChannelState) (rm.ProviderEvent, []interface{}) {
	// complete event is triggered by the actual status of completed rather than a data transfer event
	if channelState.Status() == datatransfer.Completed {
		return rm.ProviderEventComplete, nil
	}
	switch event.Code {
	case datatransfer.Accept:
		return rm.ProviderEventDealAccepted, []interface{}{channelState.ChannelID()}
	case datatransfer.Disconnected:
		return rm.ProviderEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer stalled (peer hungup)")}
	case datatransfer.Error:
		return rm.ProviderEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer failed: %s", event.Message)}
	case datatransfer.DataLimitExceeded:
		// DataLimitExceeded indicates it's time to wait for a payment
		return rm.ProviderEventPaymentRequested, nil
	case datatransfer.BeginFinalizing:
		// BeginFinalizing indicates it's time to wait for a final payment
		// Because the legacy client expects a final voucher, we dispatch this event event when
		// the deal is free -- so that we have a chance to send this final voucher before completion
		// TODO: do not send the legacy voucher when the client no longer expects it
		return rm.ProviderEventLastPaymentRequested, nil
	case datatransfer.NewVoucher:
		// NewVoucher indicates a potential new payment we should attempt to process
		return rm.ProviderEventProcessPayment, nil
	case datatransfer.Cancel:
		return rm.ProviderEventClientCancelled, nil
	default:
		return noProviderEvent, nil
	}
}

// ProviderDataTransferSubscriber is the function called when an event occurs in a data
// transfer received by a provider -- it reads the voucher to verify this event occurred
// in a storage market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func ProviderDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		voucher := channelState.Voucher()
		if voucher.Voucher == nil {
			log.Errorf("received empty voucher")
			return
		}
		dealProposal, err := rm.DealProposalFromNode(voucher.Voucher)
		// if this event is for a transfer not related to storage, ignore
		if err != nil {
			return
		}
		dealID := rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}
		retrievalEvent, params := providerEvent(event, channelState)
		if retrievalEvent == noProviderEvent {
			return
		}
		log.Debugw("processing retrieval provider dt event", "event", datatransfer.Events[event.Code], "dealID", dealProposal.ID, "peer", channelState.OtherPeer(), "retrievalEvent", rm.ProviderEvents[retrievalEvent])

		err = deals.Send(dealID, retrievalEvent, params...)
		if err != nil {
			log.Errorf("processing dt event: %s", err)
		}

	}
}

func clientEventForResponse(response *rm.DealResponse) (rm.ClientEvent, []interface{}) {
	switch response.Status {
	case rm.DealStatusRejected:
		return rm.ClientEventDealRejected, []interface{}{response.Message}
	case rm.DealStatusDealNotFound:
		return rm.ClientEventDealNotFound, []interface{}{response.Message}
	case rm.DealStatusAccepted:
		return rm.ClientEventDealAccepted, nil
	case rm.DealStatusFundsNeededUnseal:
		return rm.ClientEventUnsealPaymentRequested, []interface{}{response.PaymentOwed}
	case rm.DealStatusFundsNeededLastPayment:
		return rm.ClientEventLastPaymentRequested, []interface{}{response.PaymentOwed}
	case rm.DealStatusCompleted:
		return rm.ClientEventComplete, nil
	case rm.DealStatusFundsNeeded, rm.DealStatusOngoing:
		return rm.ClientEventPaymentRequested, []interface{}{response.PaymentOwed}
	default:
		return rm.ClientEventUnknownResponseReceived, []interface{}{response.Status}
	}
}

const noEvent = rm.ClientEvent(math.MaxUint64)

func clientEvent(event datatransfer.Event, channelState datatransfer.ChannelState) (rm.ClientEvent, []interface{}) {
	switch event.Code {
	case datatransfer.DataReceivedProgress:
		return rm.ClientEventBlocksReceived, []interface{}{channelState.Received()}
	case datatransfer.FinishTransfer:
		return rm.ClientEventAllBlocksReceived, nil
	case datatransfer.Cancel:
		return rm.ClientEventProviderCancelled, nil
	case datatransfer.NewVoucherResult:
		voucher := channelState.LastVoucherResult()
		response, err := rm.DealResponseFromNode(voucher.Voucher)
		if err != nil {
			log.Errorf("unexpected voucher result received: %s", err.Error())
			return noEvent, nil
		}

		return clientEventForResponse(response)
	case datatransfer.Disconnected:
		return rm.ClientEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer stalled (peer hungup)")}
	case datatransfer.Error:
		if channelState.Message() == datatransfer.ErrRejected.Error() {
			return rm.ClientEventDealRejected, []interface{}{"rejected for unknown reasons"}
		}
		return rm.ClientEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer failed: %s", event.Message)}
	default:
	}

	return noEvent, nil
}

// ClientDataTransferSubscriber is the function called when an event occurs in a data
// transfer initiated on the client -- it reads the voucher to verify this even occurred
// in a retrieval market deal, then, based on the data transfer event that occurred, it dispatches
// an event to the appropriate state machine
func ClientDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		voucher := channelState.Voucher()
		dealProposal, err := rm.DealProposalFromNode(voucher.Voucher)
		// if this event is for a transfer not related to retrieval, ignore
		if err != nil {
			return
		}

		retrievalEvent, params := clientEvent(event, channelState)

		if retrievalEvent == noEvent {
			return
		}
		log.Debugw("processing retrieval client dt event", "event", datatransfer.Events[event.Code], "dealID", dealProposal.ID, "peer", channelState.OtherPeer(), "retrievalEvent", rm.ClientEvents[retrievalEvent])

		// data transfer events for progress do not affect deal state
		err = deals.Send(dealProposal.ID, retrievalEvent, params...)
		if err != nil {
			log.Errorf("processing dt event %s for state %s: %s",
				datatransfer.Events[event.Code], datatransfer.Statuses[channelState.Status()], err)
		}
	}
}

// StoreGetter retrieves the store for a given id
type StoreGetter interface {
	Get(otherPeer peer.ID, dealID rm.DealID) (bstore.Blockstore, error)
}

// TransportConfigurer configurers the graphsync transport to use a custom blockstore per deal
func TransportConfigurer(thisPeer peer.ID, storeGetter StoreGetter) datatransfer.TransportConfigurer {
	return func(channelID datatransfer.ChannelID, voucher datatransfer.TypedVoucher) []datatransfer.TransportOption {
		dealProposal, err := rm.DealProposalFromNode(voucher.Voucher)
		if err != nil {
			log.Debugf("not a deal proposal voucher: %s", err.Error())
			return nil
		}
		otherPeer := channelID.OtherParty(thisPeer)
		store, err := storeGetter.Get(otherPeer, dealProposal.ID)
		if err != nil {
			log.Errorf("attempting to configure data store: %s", err)
			return nil
		}
		if store == nil {
			return nil
		}
		return []datatransfer.TransportOption{dtgs.UseStore(storeutil.LinkSystemForBlockstore(store))}
	}
}
