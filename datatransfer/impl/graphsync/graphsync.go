package graphsyncimpl

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
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
// -- actually validate requests
// -- support Push requests
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
}

type graphsyncReceiver struct {
	impl *graphsyncImpl
}

// NewGraphSyncDataTransfer initializes a new graphsync based data transfer manager
func NewGraphSyncDataTransfer(parent context.Context, host host.Host, gs graphsync.GraphExchange) datatransfer.Manager {
	dataTransferNetwork := network.NewFromLibp2pHost(host)
	impl := &graphsyncImpl{dataTransferNetwork, nil, make(map[string]validateType), make(map[datatransfer.ChannelID]datatransfer.ChannelState)}
	receiver := &graphsyncReceiver{impl}
	dataTransferNetwork.SetDelegate(receiver)
	return impl
}

// RegisterVoucherType registers a validator for the given voucher type
// will error if voucher type does not implement voucher
// or if there is a voucher type registered with an identical identifier
// This assumes that the voucherType is a pointer type, and that anything that implements datatransfer.Voucher
// Takes a pointer receiver.
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
	tid, err := impl.sendRequest(selector, false, voucher, baseCid, to)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	chid := impl.createNewChannel(tid, to, baseCid, selector, voucher)
	return chid, nil
}

// OpenPullDataChannel opens a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {
	tid, err := impl.sendRequest(selector, true, voucher, baseCid, to)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	chid := impl.createNewChannel(tid, to, baseCid, selector, voucher)
	return chid, nil
}

// createNewChannel creates a new channel id
func (impl *graphsyncImpl) createNewChannel(tid datatransfer.TransferID, to peer.ID, baseCid cid.Cid, selector ipld.Node, voucher datatransfer.Voucher) (datatransfer.ChannelID) {
	return datatransfer.ChannelID{To: to, ID: tid}
}

// sendRequest encapsulates message creation and posting to the data transfer network with the provided parameters
func (impl *graphsyncImpl) sendRequest(selector ipld.Node, isPull bool, voucher datatransfer.Voucher, baseCid cid.Cid, to peer.ID) (datatransfer.TransferID, error) {
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

	if err := impl.dataTransferNetwork.SendMessage(context.TODO(), to, req); err != nil {
		return 0, err
	}
	return tid, nil
}

func (impl *graphsyncImpl) sendResponse(isAccepted bool, to peer.ID, tid datatransfer.TransferID) (datatransfer.TransferID, error) {
	resp := message.NewResponse(tid, isAccepted)

	if err := impl.dataTransferNetwork.SendMessage(context.TODO(), to, resp); err != nil {
		return 0, err
	}
	return tid, nil
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
	for _,cb := range impl.subscribers {
		if cb != nil {
			cb(evt, cs)
		}
	}
}

// GetSubscribers returns the slice of subscribers for this graphsyncImpl
func (impl *graphsyncImpl) GetSubscribers() []datatransfer.Subscriber {
	return impl.subscribers
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

	var success bool
	// not yet doing anything else with the voucher
	_, err := receiver.validateVoucher(sender, incoming)
	if err == nil {
		success = true
	}

	// not yet doing anything else with the transfer ID
	_, err = receiver.impl.sendResponse(success, sender, incoming.TransferID())
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

func (receiver *graphsyncReceiver) ReceiveResponse(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferResponse) {
		var evt datatransfer.Event
		if !incoming.Accepted() {
			evt = datatransfer.Error
		} else {
			evt = datatransfer.Progress // for now
		}
		receiver.impl.notifySubscribers(evt, datatransfer.ChannelState{})
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
