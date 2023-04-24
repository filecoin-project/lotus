// Package dtutils provides event listeners for the client and provider to
// listen for events on the data transfer module and dispatch FSM events based on them
package dtutils

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync/storeutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	dtgs "github.com/filecoin-project/go-data-transfer/v2/transport/graphsync"
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
		node := channelState.Voucher()
		if node.Voucher == nil {
			log.Debugw("ignoring data-transfer event as it's not storage related", "event", datatransfer.Events[event.Code], "channelID",
				channelState.ChannelID())
			return
		}
		voucherIface, err := requestvalidation.BindnodeRegistry.TypeFromNode(node.Voucher, &requestvalidation.StorageDataTransferVoucher{})
		// if this event is for a transfer not related to storage, ignore
		if err != nil {
			log.Debugw("ignoring data-transfer event as it's not storage related", "event", datatransfer.Events[event.Code], "channelID",
				channelState.ChannelID())
			return
		}
		voucher, _ := voucherIface.(*requestvalidation.StorageDataTransferVoucher) // safe to assume type

		log.Debugw("processing storage provider dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
			channelState.ChannelID(), "channelState", datatransfer.Statuses[channelState.Status()])

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(voucher.Proposal, storagemarket.ProviderEventDataTransferCompleted)
			if err != nil {
				log.Errorf("processing dt event: %s", err)
				return
			}
		}

		// Translate from data transfer events to provider FSM events
		// Note: We ignore data transfer progress events (they do not affect deal state)
		err = func() error {
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
		// TODO: are these log messages valid for Client?
		node := channelState.Voucher()
		if node.Voucher == nil {
			log.Debugw("ignoring data-transfer event as it's not storage related", "event", datatransfer.Events[event.Code], "channelID",
				channelState.ChannelID())
			return
		}
		voucherIface, err := requestvalidation.BindnodeRegistry.TypeFromNode(node.Voucher, &requestvalidation.StorageDataTransferVoucher{})
		// if this event is for a transfer not related to storage, ignore
		if err != nil {
			log.Debugw("ignoring data-transfer event as it's not storage related", "event", datatransfer.Events[event.Code], "channelID",
				channelState.ChannelID())
			return
		}
		voucher, _ := voucherIface.(*requestvalidation.StorageDataTransferVoucher) // safe to assume type

		// Note: We ignore data transfer progress events (they do not affect deal state)
		log.Debugw("processing storage client dt event", "event", datatransfer.Events[event.Code], "proposalCid", voucher.Proposal, "channelID",
			channelState.ChannelID(), "channelState", datatransfer.Statuses[channelState.Status()])

		if channelState.Status() == datatransfer.Completed {
			err := deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferComplete)
			if err != nil {
				log.Errorf("processing dt event: %s", err)
				return
			}
		}

		err = func() error {
			switch event.Code {
			case datatransfer.Cancel:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferCancelled)
			case datatransfer.Restart:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferRestarted, channelState.ChannelID())
			case datatransfer.Disconnected:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferStalled)
			case datatransfer.Accept:
				return deals.Send(voucher.Proposal, storagemarket.ClientEventDataTransferQueued, channelState.ChannelID())
			case datatransfer.TransferInitiated:
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

// TransportConfigurer configurers the graphsync transport to use a custom blockstore per deal
func TransportConfigurer(storeGetter StoreGetter) datatransfer.TransportConfigurer {
	return func(channelID datatransfer.ChannelID, voucher datatransfer.TypedVoucher) []datatransfer.TransportOption {
		if voucher.Voucher == nil {
			log.Errorf("attempting to configure data store, empty voucher")
			return nil
		}
		voucherIface, err := requestvalidation.BindnodeRegistry.TypeFromNode(voucher.Voucher, &requestvalidation.StorageDataTransferVoucher{})
		// if this event is for a transfer not related to storage, ignore
		if err != nil {
			log.Errorf("attempting to configure data store, bad voucher: %s", err)
			return nil
		}
		storageVoucher, _ := voucherIface.(*requestvalidation.StorageDataTransferVoucher) // safe to assume type
		store, err := storeGetter.Get(storageVoucher.Proposal)
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
