package graphsyncimpl

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
)

const (
	// ExtensionDataTransfer is the identifier for the data transfer extension to graphsync
	ExtensionDataTransfer = graphsync.ExtensionName("fil/data-transfer")
)

// ExtensionDataTransferData is the extension data for
// the graphsync extension. TODO: feel free to add to this
type ExtensionDataTransferData struct {
	TransferID uint64
}

// This file implements a VERY simple, incomplete version of the data transfer
// module that allows us to make the necessary insertions of data transfer
// functionality into the storage market
// It does not:
// -- support multiple subscribers
// -- do any actual network coordination or use Graphsync

type validateType struct {
	voucherType reflect.Type                  // nolint: structcheck
	validator   datatransfer.RequestValidator // nolint: structcheck
}

type graphsyncImpl struct {
	dataTransferNetwork network.DataTransferNetwork
	subscribers         []datatransfer.Subscriber
	validatedTypes      map[string]validateType
	channels            map[datatransfer.ChannelID]datatransfer.ChannelState
	gs                  graphsync.GraphExchange
	peerID              peer.ID
	lastTID             int64
}

// NewGraphSyncDataTransfer initializes a new graphsync based data transfer manager
func NewGraphSyncDataTransfer(parent context.Context, host host.Host, gs graphsync.GraphExchange) datatransfer.Manager {
	dataTransferNetwork := network.NewFromLibp2pHost(host)
	impl := &graphsyncImpl{
		dataTransferNetwork,
		nil,
		make(map[string]validateType),
		make(map[datatransfer.ChannelID]datatransfer.ChannelState),
		gs,
		host.ID(),
		0,
	}
	if err := gs.RegisterRequestReceivedHook(true, impl.GsReqRecdHook); err != nil {
		log.Error(err)
		return nil
	}
	receiver := &graphsyncReceiver{parent, impl}
	dataTransferNetwork.SetDelegate(receiver)
	return impl
}

func (impl *graphsyncImpl) GsReqRecdHook(p peer.ID, request graphsync.RequestData) ([]graphsync.ExtensionData, error) {
	var resp []graphsync.ExtensionData
	data, ok := request.Extension(ExtensionDataTransfer)
	if !ok {
		return resp, errors.New("extension not present")
	}
	unm, err := impl.unmarshalExtensionData(data)
	if err != nil {
		return resp, err
	}
	chid := datatransfer.ChannelID{
		Initiator: impl.peerID,
		ID:        datatransfer.TransferID(unm.TransferID),
	}
	if !impl.HasPushChannel(chid) {
		return resp, errors.New("could not find push channel")
	}
	return resp, nil
}

func (impl *graphsyncImpl) unmarshalExtensionData(data []byte) (*ExtensionDataTransferData, error) {
	var extStruct ExtensionDataTransferData

	reader := strings.NewReader(string(data[:]))
	if err := extStruct.UnmarshalCBOR(reader); err != nil {
		return nil, err
	}
	return &extStruct, nil
}

// RegisterVoucherType registers a validator for the given voucher type
// returns error if:
// * voucher type does not implement voucher
// * there is a voucher type registered with an identical identifier
// * voucherType's Kind is not reflect.Ptr
func (impl *graphsyncImpl) RegisterVoucherType(voucherType reflect.Type, validator datatransfer.RequestValidator) error {
	if voucherType.Kind() != reflect.Ptr {
		return fmt.Errorf("voucherType must be a reflect.Ptr Kind")
	}
	v := reflect.New(voucherType.Elem())
	voucher, ok := v.Interface().(datatransfer.Voucher)
	if !ok {
		return fmt.Errorf("voucher does not implement Voucher interface")
	}

	_, isReg := impl.validatedTypes[voucher.Type()]
	if isReg {
		return fmt.Errorf("voucher type already registered: %s", voucherType.String())
	}

	impl.validatedTypes[voucher.Type()] = validateType{
		voucherType: voucherType,
		validator:   validator,
	}
	return nil
}

// OpenPushDataChannel opens a data transfer that will send data to the recipient peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPushDataChannel(ctx context.Context, requestTo peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {
	tid, err := impl.sendRequest(ctx, selector, false, voucher, baseCid, requestTo)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	// initiator = us, sender = us, receiver = them
	chid := impl.createNewChannel(tid, baseCid, selector, voucher,
		impl.peerID, impl.peerID, requestTo)
	return chid, nil
}

// OpenPullDataChannel opens a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPullDataChannel(ctx context.Context, requestTo peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {

	tid, err := impl.sendRequest(ctx, selector, true, voucher, baseCid, requestTo)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	// initiator = us, sender = them, receiver = us
	chid := impl.createNewChannel(tid, baseCid, selector, voucher,
		impl.peerID, requestTo, impl.peerID)
	return chid, nil
}

// createNewChannel creates a new channel id and channel state and saves to channels
func (impl *graphsyncImpl) createNewChannel(tid datatransfer.TransferID, baseCid cid.Cid, selector ipld.Node, voucher datatransfer.Voucher, initiator, dataSender, dataReceiver peer.ID) datatransfer.ChannelID {
	chid := datatransfer.ChannelID{Initiator: initiator, ID: tid}
	chst := datatransfer.ChannelState{Channel: datatransfer.NewChannel(0, baseCid, selector, voucher, dataSender, dataReceiver, 0)}
	impl.channels[chid] = chst
	return chid
}

// sendRequest encapsulates message creation and posting to the data transfer network with the provided parameters
func (impl *graphsyncImpl) sendRequest(ctx context.Context, selector ipld.Node, isPull bool, voucher datatransfer.Voucher, baseCid cid.Cid, to peer.ID) (datatransfer.TransferID, error) {
	sbytes, err := nodeAsBytes(selector)
	if err != nil {
		return 0, err
	}
	vbytes, err := voucher.ToBytes()
	if err != nil {
		return 0, err
	}
	tid := impl.generateTransferID()
	req := message.NewRequest(tid, isPull, voucher.Type(), vbytes, baseCid, sbytes)

	if err := impl.dataTransferNetwork.SendMessage(ctx, to, req); err != nil {
		return 0, err
	}
	return tid, nil
}

func (impl *graphsyncImpl) sendResponse(ctx context.Context, isAccepted bool, to peer.ID, tid datatransfer.TransferID) {
	resp := message.NewResponse(tid, isAccepted)
	if err := impl.dataTransferNetwork.SendMessage(ctx, to, resp); err != nil {
		log.Error(err)
	}
}

// close an open channel (effectively a cancel)
func (impl *graphsyncImpl) CloseDataTransferChannel(x datatransfer.ChannelID) {}

// get status of a transfer
func (impl *graphsyncImpl) TransferChannelStatus(x datatransfer.ChannelID) datatransfer.Status {
	return datatransfer.ChannelNotFoundError
}

// get notified when certain types of events happen
func (impl *graphsyncImpl) SubscribeToEvents(subscriber datatransfer.Subscriber) datatransfer.Unsubscribe {
	impl.subscribers = append(impl.subscribers, subscriber)
	return impl.unsubscribeAt(subscriber)
}

// unsubscribeAt returns a function that removes an item from impl.subscribers by comparing
// their reflect.ValueOf before pulling the item out of the slice.  Does not preserve order.
// Subsequent, repeated calls to the func with the same Subscriber are a no-op.
func (impl *graphsyncImpl) unsubscribeAt(sub datatransfer.Subscriber) datatransfer.Unsubscribe {
	return func() {
		curLen := len(impl.subscribers)
		for i, el := range impl.subscribers {
			if reflect.ValueOf(sub) == reflect.ValueOf(el) {
				impl.subscribers[i] = impl.subscribers[curLen-1]
				impl.subscribers = impl.subscribers[:curLen-1]
				return
			}
		}
	}
}

func (impl *graphsyncImpl) notifySubscribers(evt datatransfer.Event, cs datatransfer.ChannelState) {
	for _, cb := range impl.subscribers {
		cb(evt, cs)
	}
}

// get all in progress transfers
func (impl *graphsyncImpl) InProgressChannels() map[datatransfer.ChannelID]datatransfer.ChannelState {
	return impl.channels
}

// HasPushChannel returns true if a channel with ID chid exists and is for a Push request.
func (impl *graphsyncImpl) HasPushChannel(chid datatransfer.ChannelID) bool {
	return impl.getPushChannel(chid) != datatransfer.EmptyChannelState
}

// HasPullChannel returns true if a channel with ID chid exists and is for a Pull request.
func (impl *graphsyncImpl) HasPullChannel(chid datatransfer.ChannelID) bool {
	return impl.getPullChannel(chid) != datatransfer.EmptyChannelState
}

// getPullChannel searches for a pull-type channel in the slice of channels with id `chid`.
// Returns datatransfer.EmptyChannelState if:
//   * there is no channel with that id
//   * it is not related to a pull request
func (impl *graphsyncImpl) getPullChannel(chid datatransfer.ChannelID) datatransfer.ChannelState {
	channelState, ok := impl.channels[chid]
	if !ok || channelState.Sender() == impl.peerID {
		return datatransfer.EmptyChannelState
	}
	return channelState
}

func (impl *graphsyncImpl) getPushChannel(chid datatransfer.ChannelID) datatransfer.ChannelState {
	channelState, ok := impl.channels[chid]
	if !ok || channelState.Recipient() == impl.peerID {
		return datatransfer.EmptyChannelState
	}
	return channelState
}

// generateTransferID() generates a unique-to-runtime TransferID for use in creating
// ChannelIDs
func (impl *graphsyncImpl) generateTransferID() datatransfer.TransferID {
	impl.lastTID++
	return datatransfer.TransferID(impl.lastTID)
}
