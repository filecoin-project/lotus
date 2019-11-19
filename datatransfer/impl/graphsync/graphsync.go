package graphsyncimpl

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	logging "github.com/ipfs/go-log"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
)

var log = logging.Logger("graphsync-impl")

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
}

type graphsyncReceiver struct {
	ctx  context.Context
	impl *graphsyncImpl
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
	}
	receiver := &graphsyncReceiver{parent, impl}
	dataTransferNetwork.SetDelegate(receiver)
	return impl
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
func (impl *graphsyncImpl) OpenPushDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {
	tid, err := impl.sendRequest(ctx, selector, false, voucher, baseCid, to)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	chid := impl.createNewChannel(tid, baseCid, selector, voucher, to, "", to)
	return chid, nil
}

// OpenPullDataChannel opens a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {

	tid, err := impl.sendRequest(ctx, selector, true, voucher, baseCid, to)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	chid := impl.createNewChannel(tid, baseCid, selector, voucher, to, to, "")
	return chid, nil
}

// createNewChannel creates a new channel id and channel state and saves to channels
func (impl *graphsyncImpl) createNewChannel(tid datatransfer.TransferID, baseCid cid.Cid, selector ipld.Node, voucher datatransfer.Voucher, to, sender, receiver peer.ID) datatransfer.ChannelID {
	chid := datatransfer.ChannelID{To: to, ID: tid}
	chst := datatransfer.ChannelState{Channel: datatransfer.NewChannel(0, baseCid, selector, voucher, sender, receiver, 0)}
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
	return nil
}

// ReceiveRequest takes an incoming request, validates the voucher and processes the message.
func (receiver *graphsyncReceiver) ReceiveRequest(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferRequest) {

	// not yet doing anything else with the voucher
	_, err := receiver.validateVoucher(sender, incoming)
	if err != nil {
		receiver.impl.sendResponse(ctx, false, sender, incoming.TransferID())
		return
	}
	stor, _ := nodeFromBytes(incoming.Selector())
	root := cidlink.Link{incoming.BaseCid()}
	if !incoming.IsPull() {
		receiver.impl.gs.Request(ctx, sender, root, stor)
	}
	receiver.impl.sendResponse(ctx, true, sender, incoming.TransferID())
}

// validateVoucher converts a voucher in an incoming message to its appropriate
// voucher struct, then runs the validator and returns the results.
// returns error if:
//   * voucherFromRequest fails
//   * deserialization of selector fails
//   * validation fails
func (receiver *graphsyncReceiver) validateVoucher(sender peer.ID, incoming message.DataTransferRequest) (datatransfer.Voucher, error) {

	vtypStr := incoming.VoucherType()
	vouch, err := receiver.voucherFromRequest(incoming)
	if err != nil {
		return vouch, err
	}

	var validatorFunc func(peer.ID, datatransfer.Voucher, cid.Cid, ipld.Node) error
	if incoming.IsPull() {
		validatorFunc = receiver.impl.validatedTypes[vtypStr].validator.ValidatePull
	} else {
		validatorFunc = receiver.impl.validatedTypes[vtypStr].validator.ValidatePush
	}

	stor, err := nodeFromBytes(incoming.Selector())
	if err != nil {
		return vouch, err
	}

	if err = validatorFunc(sender, vouch, incoming.BaseCid(), stor); err != nil {
		return nil, err
	}

	return vouch, nil
}

// voucherFromRequest takes an incoming request and attempts to create a
// voucher struct from it using the registered validated types.  It returns
// a deserialized voucher and any error.  It returns error if:
//    * the voucher type has no validator registered
//    * the voucher cannot be instantiated via reflection
//    * request voucher bytes cannot be deserialized via <voucher>.FromBytes()
func (receiver *graphsyncReceiver) voucherFromRequest(incoming message.DataTransferRequest) (datatransfer.Voucher, error) {
	vtypStr := incoming.VoucherType()

	validatedType, ok := receiver.impl.validatedTypes[vtypStr]
	if !ok {
		return nil, fmt.Errorf("unregistered voucher type %s", vtypStr)
	}
	vStructVal := reflect.New(validatedType.voucherType.Elem())
	voucher, ok := vStructVal.Interface().(datatransfer.Voucher)
	if !ok || reflect.ValueOf(voucher).IsNil() {
		return nil, fmt.Errorf("problem instantiating type %s, voucher: %v", vtypStr, voucher)
	}
	if err := voucher.FromBytes(incoming.Voucher()); err != nil {
		return voucher, err
	}
	return voucher, nil
}

// ReceiveResponse handles responding to Push or Pull Requests.
// It schedules a graphsync transfer only if a Pull Request is accepted.
func (receiver *graphsyncReceiver) ReceiveResponse(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferResponse) {
	evt := datatransfer.Error
	chst := datatransfer.EmptyChannelState
	if incoming.Accepted() {
		chid := datatransfer.ChannelID{
			To: sender,
			ID: incoming.TransferID(),
		}
		if chst = receiver.impl.getPullChannel(chid); chst != datatransfer.EmptyChannelState {
			baseCid := chst.BaseCID()
			root := cidlink.Link{baseCid}
			receiver.impl.gs.Request(ctx, sender, root, chst.Selector())
			evt = datatransfer.Progress
		}
	}
	receiver.impl.notifySubscribers(evt, chst)
}

// getPullChannel searches for a pull-type channel in the slice of channels with id `chid`.
// Returns datatransfer.EmptyChannelState if:
//   * there is no channel with that id
//   * it is not related to a pull request
func (impl *graphsyncImpl) getPullChannel(chid datatransfer.ChannelID) datatransfer.ChannelState {
	channelState, ok := impl.channels[chid]
	if !ok || channelState.Sender() == "" {
		return datatransfer.EmptyChannelState
	}
	return channelState
}

func (receiver *graphsyncReceiver) ReceiveError(error) {}

// nodeAsBytes serializes an ipld.Node
func nodeAsBytes(node ipld.Node) ([]byte, error) {
	var buffer bytes.Buffer
	err := dagcbor.Encoder(node, &buffer)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// nodeFromBytes deserializes an ipld.Node
func nodeFromBytes(from []byte) (ipld.Node, error) {
	reader := bytes.NewReader(from)
	return dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
}

// TODO: implement a real transfer ID generator.
// https://github.com/filecoin-project/go-data-transfer/issues/38
func (impl *graphsyncImpl) generateTransferID() datatransfer.TransferID {
	return datatransfer.TransferID(rand.Int31())
}
