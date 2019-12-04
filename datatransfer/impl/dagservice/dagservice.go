package datatransfer

import (
	"context"
	"reflect"
	"time"

	"github.com/ipfs/go-cid"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/datatransfer"
)

// This file implements a VERY simple, incomplete version of the data transfer
// module that allows us to make the necessary insertions of data transfer
// functionality into the storage market
// It does not:
// -- actually validate requests
// -- support Push requests
// -- support multiple subscribers
// -- do any actual network coordination or use Graphsync

type dagserviceImpl struct {
	dag        ipldformat.DAGService
	subscriber datatransfer.Subscriber
}

// NewDAGServiceDataTransfer returns a data transfer manager based on
// an IPLD DAGService
func NewDAGServiceDataTransfer(dag ipldformat.DAGService) datatransfer.Manager {
	return &dagserviceImpl{dag, nil}
}

// RegisterVoucherType registers a validator for the given voucher type
// will error if voucher type does not implement voucher
// or if there is a voucher type registered with an identical identifier
func (impl *dagserviceImpl) RegisterVoucherType(voucherType reflect.Type, validator datatransfer.RequestValidator) error {
	return nil
}

// open a data transfer that will send data to the recipient peer and
// transfer parts of the piece that match the selector
func (impl *dagserviceImpl) OpenPushDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, Selector ipld.Node) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, xerrors.Errorf("not implemented")
}

// open a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *dagserviceImpl) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, Selector ipld.Node) (datatransfer.ChannelID, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		err := merkledag.FetchGraph(ctx, baseCid, impl.dag)
		event := datatransfer.Event{Timestamp: time.Now()}
		if err != nil {
			event.Code = datatransfer.Error
			event.Message = err.Error()
		} else {
			event.Code = datatransfer.Complete
		}
		impl.subscriber(event, datatransfer.ChannelState{Channel: datatransfer.NewChannel(0, baseCid, Selector, voucher, to, "", 0)})
	}()
	return datatransfer.ChannelID{}, nil
}

// close an open channel (effectively a cancel)
func (impl *dagserviceImpl) CloseDataTransferChannel(x datatransfer.ChannelID) {}

// get status of a transfer
func (impl *dagserviceImpl) TransferChannelStatus(x datatransfer.ChannelID) datatransfer.Status {
	return datatransfer.ChannelNotFoundError
}

// get notified when certain types of events happen
func (impl *dagserviceImpl) SubscribeToEvents(subscriber datatransfer.Subscriber) datatransfer.Unsubscribe {
	impl.subscriber = subscriber
	return func() {}
}

// get all in progress transfers
func (impl *dagserviceImpl) InProgressChannels() map[datatransfer.ChannelID]datatransfer.ChannelState {
	return nil
}
