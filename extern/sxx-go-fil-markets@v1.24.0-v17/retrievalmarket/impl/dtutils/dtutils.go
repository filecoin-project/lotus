// Package dtutils provides event listeners for the client and provider to
// listen for events on the data transfer module and dispatch FSM events based on them
package dtutils

import (
	"fmt"
	"math"

	"github.com/ipfs/go-graphsync/storeutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"

	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
)

var log = logging.Logger("retrievalmarket_impl")

// EventReceiver is any thing that can receive FSM events
type EventReceiver interface {
	Send(id interface{}, name fsm.EventName, args ...interface{}) (err error)
}

const noProviderEvent = rm.ProviderEvent(math.MaxUint64)

func providerEvent(event datatransfer.Event, channelState datatransfer.ChannelState) (rm.ProviderEvent, []interface{}) {
	switch event.Code {
	case datatransfer.Accept:
		return rm.ProviderEventDealAccepted, []interface{}{channelState.ChannelID()}
	case datatransfer.Disconnected:
		return rm.ProviderEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer stalled (peer hungup)")}
	case datatransfer.Error:
		return rm.ProviderEventDataTransferError, []interface{}{fmt.Errorf("deal data transfer failed: %s", event.Message)}
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
		dealProposal, ok := dealProposalFromVoucher(channelState.Voucher())
		// if this event is for a transfer not related to storage, ignore
		if !ok {
			return
		}

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}, rm.ProviderEventComplete)
			if err != nil {
				log.Errorf("processing dt event: %s", err)
			}
		}

		retrievalEvent, params := providerEvent(event, channelState)
		if retrievalEvent == noProviderEvent {
			return
		}
		log.Debugw("processing retrieval provider dt event", "event", datatransfer.Events[event.Code], "dealID", dealProposal.ID, "peer", channelState.OtherPeer(), "retrievalEvent", rm.ProviderEvents[retrievalEvent])

		err := deals.Send(rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: channelState.Recipient()}, retrievalEvent, params...)
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
		response, ok := dealResponseFromVoucherResult(channelState.LastVoucherResult())
		if !ok {
			log.Errorf("unexpected voucher result received: %s", channelState.LastVoucher().Type())
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
		dealProposal, ok := dealProposalFromVoucher(channelState.Voucher())

		// if this event is for a transfer not related to retrieval, ignore
		if !ok {
			return
		}

		retrievalEvent, params := clientEvent(event, channelState)

		if retrievalEvent == noEvent {
			return
		}
		log.Debugw("processing retrieval client dt event", "event", datatransfer.Events[event.Code], "dealID", dealProposal.ID, "peer", channelState.OtherPeer(), "retrievalEvent", rm.ClientEvents[retrievalEvent])

		// data transfer events for progress do not affect deal state
		err := deals.Send(dealProposal.ID, retrievalEvent, params...)
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

// StoreConfigurableTransport defines the methods needed to
// configure a data transfer transport use a unique store for a given request
type StoreConfigurableTransport interface {
	UseStore(datatransfer.ChannelID, ipld.LinkSystem) error
}

// TransportConfigurer configurers the graphsync transport to use a custom blockstore per deal
func TransportConfigurer(thisPeer peer.ID, storeGetter StoreGetter) datatransfer.TransportConfigurer {
	return func(channelID datatransfer.ChannelID, voucher datatransfer.Voucher, transport datatransfer.Transport) {
		dealProposal, ok := dealProposalFromVoucher(voucher)
		if !ok {
			return
		}
		gsTransport, ok := transport.(StoreConfigurableTransport)
		if !ok {
			return
		}
		otherPeer := channelID.OtherParty(thisPeer)
		store, err := storeGetter.Get(otherPeer, dealProposal.ID)
		if err != nil {
			log.Errorf("attempting to configure data store: %s", err)
			return
		}
		if store == nil {
			return
		}
		err = gsTransport.UseStore(channelID, storeutil.LinkSystemForBlockstore(store))
		if err != nil {
			log.Errorf("attempting to configure data store: %s", err)
		}
	}
}

func dealProposalFromVoucher(voucher datatransfer.Voucher) (*rm.DealProposal, bool) {
	dealProposal, ok := voucher.(*rm.DealProposal)
	// if this event is for a transfer not related to storage, ignore
	if ok {
		return dealProposal, true
	}

	legacyProposal, ok := voucher.(*migrations.DealProposal0)
	if !ok {
		return nil, false
	}
	newProposal := migrations.MigrateDealProposal0To1(*legacyProposal)
	return &newProposal, true
}

func dealResponseFromVoucherResult(vres datatransfer.VoucherResult) (*rm.DealResponse, bool) {
	dealResponse, ok := vres.(*rm.DealResponse)
	// if this event is for a transfer not related to storage, ignore
	if ok {
		return dealResponse, true
	}

	legacyResponse, ok := vres.(*migrations.DealResponse0)
	if !ok {
		return nil, false
	}
	newResponse := migrations.MigrateDealResponse0To1(*legacyResponse)
	return &newResponse, true
}
