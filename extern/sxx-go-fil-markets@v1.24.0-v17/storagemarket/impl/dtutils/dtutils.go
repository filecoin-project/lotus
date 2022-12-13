// Package dtutils provides event listeners for the client and provider to
// listen for events on the data transfer module and dispatch FSM events based on them
package dtutils

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync/storeutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

var log = logging.Logger("storagemarket_impl")

// EventReceiver is any thing that can receive FSM events
type EventReceiver interface {
	Send(id interface{}, name fsm.EventName, args ...interface{}) (err error)
}

// ProviderDataTransferSubscriber is the function called when an event occurs in a data
// transfer received by a provider -- it reads the voucher to verify this event occurred
// in a storage market deal, then, based on the data transfer event that occurred, it generates
// and update message for the deal -- either moving to staged for a completion
// event or moving to error if a data transfer error occurs
func ProviderDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		voucher, ok := channelState.Voucher().(*requestvalidation.StorageDataTransferVoucher)
		// if this event is for a transfer not related to storage, ignore
		if !ok {
			log.Debugw("ignoring data-transfer event as it's not storage related", "event", datatransfer.Events[event.Code], "channelID",
				channelState.ChannelID())
			return
		}

		log.Debugw("processing storage provider dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
			channelState.ChannelID(), "channelState", datatransfer.Statuses[channelState.Status()])

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferCompleted)
			if err != nil {
				log.Errorf("processing dt event: %s", err)
			}
		}

		// Translate from data transfer events to provider FSM events
		// Note: We ignore data transfer progress events (they do not affect deal state)
		err := func() error {
			switch event.Code {
			case datatransfer.Cancel:
				return deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferCancelled)
			case datatransfer.Restart:
				return deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferRestarted, channelState.ChannelID())
			case datatransfer.Disconnected:
				return deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferStalled)
			case datatransfer.Open:
				return deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferInitiated, channelState.ChannelID())
			case datatransfer.Error:
				return deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferFailed, fmt.Errorf("deal data transfer failed: %s", event.Message))
			default:
				return nil
			}
		}()
		if err != nil {
			log.Errorw("error processing storage provider dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
				channelState.ChannelID(), "err", err)
		}
	}
}

// ClientDataTransferSubscriber is the function called when an event occurs in a data
// transfer initiated on the client -- it reads the voucher to verify this even occurred
// in a storage market deal, then, based on the data transfer event that occurred, it dispatches
// an event to the appropriate state machine
func ClientDataTransferSubscriber(deals EventReceiver) datatransfer.Subscriber {
	return func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		voucher, ok := channelState.Voucher().(*requestvalidation.StorageDataTransferVoucher)
		// if this event is for a transfer not related to storage, ignore
		if !ok {
			return
		}

		// Note: We ignore data transfer progress events (they do not affect deal state)
		log.Debugw("processing storage client dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
			channelState.ChannelID(), "channelState", datatransfer.Statuses[channelState.Status()])

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferComplete)
			if err != nil {
				log.Errorf("processing dt event: %s", err)
			}
		}

		err := func() error {
			switch event.Code {
			case datatransfer.Cancel:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferCancelled)
			case datatransfer.Restart:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferRestarted, channelState.ChannelID())
			case datatransfer.Disconnected:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferStalled)
			case datatransfer.TransferRequestQueued:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferQueued, channelState.ChannelID())
			case datatransfer.Accept:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferInitiated, channelState.ChannelID())
			case datatransfer.Error:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferFailed, fmt.Errorf("deal data transfer failed: %s", event.Message))
			default:
				return nil
			}
		}()
		if err != nil {
			log.Errorw("error processing storage client dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
				channelState.ChannelID(), "err", err)
		}
	}
}

// StoreGetter retrieves the store for a given proposal cid
type StoreGetter interface {
	Get(proposalCid cid.Cid) (bstore.Blockstore, error)
}

// StoreConfigurableTransport defines the methods needed to
// configure a data transfer transport use a unique store for a given request
type StoreConfigurableTransport interface {
	UseStore(datatransfer.ChannelID, ipld.LinkSystem) error
}

// TransportConfigurer configurers the graphsync transport to use a custom blockstore per deal
func TransportConfigurer(storeGetter StoreGetter) datatransfer.TransportConfigurer {
	return func(channelID datatransfer.ChannelID, voucher datatransfer.Voucher, transport datatransfer.Transport) {
		storageVoucher, ok := voucher.(*requestvalidation.StorageDataTransferVoucher)
		if !ok {
			return
		}
		gsTransport, ok := transport.(StoreConfigurableTransport)
		if !ok {
			return
		}
		store, err := storeGetter.Get(storageVoucher.Proposal)
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
