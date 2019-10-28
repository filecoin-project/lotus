package datatransfer

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	ipld "github.com/ipld/go-ipld-prime"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type dagserviceImpl struct {
	dag        ipldformat.DAGService
	subscriber Subscriber
}

func NewProviderDAGServiceDataTransfer(dag dtypes.StagingDAG) Manager {
	return &dagserviceImpl{dag, nil}
}

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
func (impl *dagserviceImpl) OpenPushDataChannel(to peer.ID, voucher Voucher, PieceRef cid.Cid, Selector ipld.Node) (ChannelID, error) {
	return ChannelID{}, fmt.Errorf("not implemented")
}

// open a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *dagserviceImpl) OpenPullDataChannel(to peer.ID, voucher Voucher, PieceRef cid.Cid, Selector ipld.Node) (ChannelID, error) {
	err := merkledag.FetchGraph(context.TODO(), PieceRef, impl.dag)
	var event Event
	if err != nil {
		event = Error
	} else {
		event = Complete
	}
	impl.subscriber(event, ChannelState{Channel: Channel{voucher: voucher}})
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
