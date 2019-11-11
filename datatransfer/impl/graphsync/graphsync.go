package graphsync

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"reflect"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	"github.com/libp2p/go-libp2p-core/host"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/datatransfer"
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

	v := reflect.Indirect(reflect.New(voucherType)).Interface()
	voucher, ok := v.(datatransfer.Voucher)
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
	return datatransfer.ChannelID{To: to, ID: tid}, nil
}

// OpenPullDataChannel opens a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPullDataChannel(ctx context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {
	tid, err := impl.sendRequest(selector, true, voucher, baseCid, to)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	return datatransfer.ChannelID{To: to, ID: tid}, nil
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
// ReceiveRequest takes an incoming request, validates the voucher and processes the message.
func (receiver *graphsyncReceiver) ReceiveRequest(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferRequest) {

	voucher, err := receiver.validateVoucher(sender, incoming)
	if err != nil {
		fmt.Printf(err.Error())
	}
	if voucher == nil {
		fmt.Printf("\nincoming voucher failed to validate: %v\n", incoming)
	}
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
		return vouch,err
	}

	var validatorFunc func(peer.ID, datatransfer.Voucher, cid.Cid, ipld.Node) error
	if incoming.IsPull() {
		validatorFunc = receiver.impl.validatedTypes[vtypStr].validator.ValidatePull
	} else {
		validatorFunc = receiver.impl.validatedTypes[vtypStr].validator.ValidatePush
	}

	stor, err := nodeFromBytes(incoming.Selector())
	if err != nil { return vouch, err }

	if err = validatorFunc(sender, vouch, incoming.BaseCid(), stor) ; err != nil { return nil, err	}

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
func nodeFromBytes(from []byte) (ipld.Node,error) {
	reader := bytes.NewReader(from)
	return dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
}

// TODO: implement a real transfer ID generator.
// https://github.com/filecoin-project/go-data-transfer/issues/38
func (impl *graphsyncImpl) generateTransferID() datatransfer.TransferID {
	return datatransfer.TransferID(rand.Int31())
}
