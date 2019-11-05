package datatransfer

import (
	"context"
	"reflect"

	"github.com/ipfs/go-cid"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// This file implements a VERY simple, incomplete version of the data transfer
// module that allows us to make the neccesary insertions of data transfer
// functionality into the storage market
// It does not:
// -- actually validate requests
// -- support Push requests
// -- support multiple subscribers
// -- do any actual network coordination or use Graphsync

type dagserviceImpl struct {
	dag        ipldformat.DAGService
	subscriber Subscriber
}

// NewProviderDAGServiceDataTransfer returns a data transfer manager that just
// uses the provider's Staging DAG service for transfers
func NewProviderDAGServiceDataTransfer(dag dtypes.StagingDAG) Manager {
	return &dagserviceImpl{dag, nil}
}

// NewClientDAGServiceDataTransfer returns a data transfer manager that just
// uses the clients's Client DAG service for transfers
func NewClientDAGServiceDataTransfer(dag dtypes.ClientDAG) Manager {
	return &dagserviceImpl{dag, nil}
}

// RegisterVoucherType registers a validator for the given voucher type
// will error if voucher type does not implement voucher
// or if there is a voucher type registered with an identical identifier
func (impl *dagserviceImpl) RegisterVoucherType(voucherType reflect.Type, validator RequestValidator) error {
	return nil
}

// open a data transfer that will send data to the recipient peer and
// open a data transfer that will send data to the recipient peer and
// transfer parts of the piece that match the selector
func (impl *dagserviceImpl) OpenPushDataChannel(ctx context.Context, to peer.ID, voucher Voucher, baseCid cid.Cid, Selector ipld.Node) (ChannelID, error) {
	return ChannelID{}, xerrors.Errorf("not implemented")
}

// open a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *dagserviceImpl) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher Voucher, baseCid cid.Cid, Selector ipld.Node) (ChannelID, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		err := merkledag.FetchGraph(ctx, baseCid, impl.dag)
		var event Event
		if err != nil {
			event = Error
		} else {
			event = Complete
		}
		impl.subscriber(event, ChannelState{Channel: Channel{voucher: voucher}})
	}()
	return ChannelID{}, nil
}

// close an open channel (effectively a cancel)
func (impl *dagserviceImpl) CloseDataTransferChannel(x ChannelID) {}

// get status of a transfer
func (impl *dagserviceImpl) TransferChannelStatus(x ChannelID) Status { return ChannelNotFoundError }

// get notified when certain types of events happen
func (impl *dagserviceImpl) SubscribeToEvents(subscriber Subscriber) {
	impl.subscriber = subscriber
}

// get all in progress transfers
func (impl *dagserviceImpl) InProgressChannels() map[ChannelID]ChannelState { return nil }
