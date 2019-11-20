package graphsyncimpl

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
)

type graphsyncReceiver struct {
	ctx  context.Context
	impl *graphsyncImpl
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
			Initiator: receiver.impl.peerID,
			ID:        incoming.TransferID(),
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

func (receiver *graphsyncReceiver) ReceiveError(error) {}
