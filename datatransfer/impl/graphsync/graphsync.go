package graphsync

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	datatransfer "github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
)

// This file implements a VERY simple, incomplete version of the data transfer
// module that allows us to make the necessary insertions of data transfer
// functionality into the storage market
// It does not:
// -- actually validate requests
// -- support Push requests
// -- support multiple subscribers
// -- do any actual network coordination or use Graphsync

type validateType struct {
	voucherType reflect.Type
	validator   datatransfer.RequestValidator
}

type graphsyncImpl struct {
	dataTransferNetwork network.DataTransferNetwork
	subscribers         []datatransfer.Subscriber
	validatedTypes      map[string]validateType
	channels            map[datatransfer.ChannelID]datatransfer.ChannelState
}

type graphsyncReceiver struct {
	impl *graphsyncImpl
}

// NewGraphSyncDataTransfer initializes a new graphsync based data transfer manager
func NewGraphSyncDataTransfer(parent context.Context, host host.Host, bs bstore.Blockstore) datatransfer.Manager {
	dataTransferNetwork := network.NewFromLibp2pHost(host)
	impl := &graphsyncImpl{dataTransferNetwork, nil, make(map[string]validateType), make(map[datatransfer.ChannelID]datatransfer.ChannelState)}
	receiver := &graphsyncReceiver{impl}
	dataTransferNetwork.SetDelegate(receiver)
	return impl
}

// RegisterVoucherType registers a validator for the given voucher type
// will error if voucher type does not implement voucher
// or if there is a voucher type registered with an identical identifier
// TODO: implement for https://github.com/filecoin-project/go-data-transfer/issues/15
func (impl *graphsyncImpl) RegisterVoucherType(voucherType reflect.Type, validator datatransfer.RequestValidator) error {
	return nil
}

// open a data transfer that will send data to the recipient peer and
// open a data transfer that will send data to the recipient peer and
// transfer parts of the piece that match the selector
// TODO: implement for https://github.com/filecoin-project/go-data-transfer/issues/13
func (impl *graphsyncImpl) OpenPushDataChannel(to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, Selector ipld.Node) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, fmt.Errorf("not implemented")
}

// open a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
// TODO: implement for https://github.com/filecoin-project/go-data-transfer/issues/16
func (impl *graphsyncImpl) OpenPullDataChannel(to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, Selector ipld.Node) (datatransfer.ChannelID, error) {
	return datatransfer.ChannelID{}, nil
}

// close an open channel (effectively a cancel)
func (impl *graphsyncImpl) CloseDataTransferChannel(x datatransfer.ChannelID) {}

// get status of a transfer
func (impl *graphsyncImpl) TransferChannelStatus(x datatransfer.ChannelID) datatransfer.Status {
	return datatransfer.ChannelNotFoundError
}

// get notified when certain types of events happen
func (impl *graphsyncImpl) SubscribeToEvents(subscriber datatransfer.Subscriber) datatransfer.Unsubscribe {
	return func() {}
}

// get all in progress transfers
func (impl *graphsyncImpl) InProgressChannels() map[datatransfer.ChannelID]datatransfer.ChannelState {
	return nil
}

// TODO: implement for https://github.com/filecoin-project/go-data-transfer/issues/14
func (receiver *graphsyncReceiver) ReceiveRequest(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferRequest) {
}

func (receiver *graphsyncReceiver) ReceiveResponse(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferResponse) {
}

func (receiver *graphsyncReceiver) ReceiveError(error) {}
