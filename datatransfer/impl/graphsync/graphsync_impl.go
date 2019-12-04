package graphsyncimpl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
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
	Initiator  peer.ID
	IsPull     bool
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
	channelsLk          sync.RWMutex
	channels            map[datatransfer.ChannelID]datatransfer.ChannelState
	gs                  graphsync.GraphExchange
	peerID              peer.ID
	lastTIDLk           sync.Mutex
	lastTID             int64
}

// NewGraphSyncDataTransfer initializes a new graphsync based data transfer manager
func NewGraphSyncDataTransfer(parent context.Context, host host.Host, gs graphsync.GraphExchange) datatransfer.Manager {
	dataTransferNetwork := network.NewFromLibp2pHost(host)
	impl := &graphsyncImpl{
		dataTransferNetwork,
		nil,
		make(map[string]validateType),
		sync.RWMutex{},
		make(map[datatransfer.ChannelID]datatransfer.ChannelState),
		gs,
		host.ID(),
		sync.Mutex{},
		0,
	}
	if err := gs.RegisterRequestReceivedHook(true, impl.gsReqRecdHook); err != nil {
		log.Error(err)
		return nil
	}
	dtReceiver := &graphsyncReceiver{parent, impl}
	dataTransferNetwork.SetDelegate(dtReceiver)
	return impl
}

// gsReqRecdHook is a graphsync.OnRequestReceivedHook hook
// if an incoming request does not match a previous push request, it returns an error.
func (impl *graphsyncImpl) gsReqRecdHook(p peer.ID, request graphsync.RequestData) ([]graphsync.ExtensionData, error) {
	var resp []graphsync.ExtensionData

	// if this is a push request the sender is us.
	transferData, err := impl.getExtensionData(request)
	if err != nil {
		return resp, err
	}
	raw, _ := request.Extension(ExtensionDataTransfer)
	respData := graphsync.ExtensionData{Name: ExtensionDataTransfer, Data: raw}
	resp = append(resp, respData)

	sender := impl.peerID
	initiator := impl.peerID
	if transferData.IsPull {
		// if it's a pull request: the initiator is them
		initiator = p
	}
	chid := datatransfer.ChannelID{Initiator: initiator, ID: datatransfer.TransferID(transferData.TransferID)}

	if impl.getChannelByIDAndSender(chid, sender) == datatransfer.EmptyChannelState {
		return resp, errors.New("could not find push or pull channel")
	}

	return resp, nil
}

// gsExtended is a small interface used by getExtensionData
type gsExtended interface {
	Extension(name graphsync.ExtensionName) ([]byte, bool)
}

// getExtensionData unmarshals extension data. Returns any errors.
func (impl *graphsyncImpl) getExtensionData(extendedData gsExtended) (*ExtensionDataTransferData, error) {
	data, ok := extendedData.Extension(ExtensionDataTransfer)
	if !ok {
		return nil, errors.New("extension not present")
	}
	var extStruct ExtensionDataTransferData

	reader := bytes.NewReader(data)
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
	tid, err := impl.sendDtRequest(ctx, selector, false, voucher, baseCid, requestTo)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}

	chid, err := impl.createNewChannel(tid, baseCid, selector, voucher,
		impl.peerID, impl.peerID, requestTo) // initiator = us, sender = us, receiver = them
	if err != nil {
		return chid, err
	}
	return chid, nil
}

// OpenPullDataChannel opens a data transfer that will request data from the sending peer and
// transfer parts of the piece that match the selector
func (impl *graphsyncImpl) OpenPullDataChannel(ctx context.Context, requestTo peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {

	tid, err := impl.sendDtRequest(ctx, selector, true, voucher, baseCid, requestTo)
	if err != nil {
		return datatransfer.ChannelID{}, err
	}
	// initiator = us, sender = them, receiver = us
	chid, err := impl.createNewChannel(tid, baseCid, selector, voucher,
		impl.peerID, requestTo, impl.peerID)
	if err != nil {
		return chid, err
	}
	return chid, nil
}

// createNewChannel creates a new channel id and channel state and saves to channels.
// returns error if the channel exists already.
func (impl *graphsyncImpl) createNewChannel(tid datatransfer.TransferID, baseCid cid.Cid, selector ipld.Node, voucher datatransfer.Voucher, initiator, dataSender, dataReceiver peer.ID) (datatransfer.ChannelID, error) {
	chid := datatransfer.ChannelID{Initiator: initiator, ID: tid}
	chst := datatransfer.ChannelState{Channel: datatransfer.NewChannel(0, baseCid, selector, voucher, dataSender, dataReceiver, 0)}
	impl.channelsLk.Lock()
	defer impl.channelsLk.Unlock()
	_, ok := impl.channels[chid]
	if ok {
		return chid, errors.New("tried to create channel but it already exists")
	}
	impl.channels[chid] = chst
	return chid, nil
}

// sendDtRequest encapsulates message creation and posting to the data transfer network with the provided parameters
func (impl *graphsyncImpl) sendDtRequest(ctx context.Context, selector ipld.Node, isPull bool, voucher datatransfer.Voucher, baseCid cid.Cid, to peer.ID) (datatransfer.TransferID, error) {
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
	impl.channelsLk.RLock()
	defer impl.channelsLk.RUnlock()
	channelsCopy := make(map[datatransfer.ChannelID]datatransfer.ChannelState, len(impl.channels))
	for channelID, channelState := range impl.channels {
		channelsCopy[channelID] = channelState
	}
	return channelsCopy
}

// getChannelByIDAndSender searches for a channel in the slice of channels with id `chid`.
// Returns datatransfer.EmptyChannelState if there is no channel with that id
func (impl *graphsyncImpl) getChannelByIDAndSender(chid datatransfer.ChannelID, sender peer.ID) datatransfer.ChannelState {
	impl.channelsLk.RLock()
	channelState, ok := impl.channels[chid]
	impl.channelsLk.RUnlock()
	if !ok || channelState.Sender() != sender {
		return datatransfer.EmptyChannelState
	}
	return channelState
}

// generateTransferID() generates a unique-to-runtime TransferID for use in creating
// ChannelIDs
func (impl *graphsyncImpl) generateTransferID() datatransfer.TransferID {
	impl.lastTIDLk.Lock()
	impl.lastTID++
	impl.lastTIDLk.Unlock()
	return datatransfer.TransferID(impl.lastTID)
}

// sendGsRequest assembles a graphsync request and determines if the transfer was completed/successful.
// notifies subscribers of final request status.
func (impl *graphsyncImpl) sendGsRequest(ctx context.Context, initiator peer.ID, transferID datatransfer.TransferID, isPull bool, dataSender peer.ID, root cidlink.Link, stor ipld.Node) {
	extDtData := ExtensionDataTransferData{
		TransferID: uint64(transferID),
		Initiator:  initiator,
		IsPull:     isPull,
	}
	var buf bytes.Buffer
	if err := extDtData.MarshalCBOR(&buf); err != nil {
		log.Error(err)
	}
	extData := buf.Bytes()
	_, errChan := impl.gs.Request(ctx, dataSender, root, stor,
		graphsync.ExtensionData{
			Name: ExtensionDataTransfer,
			Data: extData,
		})
	go func() {
		var lastError error
		for err := range errChan {
			lastError = err
		}
		evt := datatransfer.Event{
			Code:      datatransfer.Error,
			Timestamp: time.Now(),
		}
		chid := datatransfer.ChannelID{Initiator: initiator, ID: transferID}
		chst := impl.getChannelByIDAndSender(chid, dataSender)
		if chst == datatransfer.EmptyChannelState {
			msg := "cannot find a matching channel for this request"
			evt.Message = msg
		} else {
			if lastError == nil {
				evt.Code = datatransfer.Complete
			} else {
				evt.Message = lastError.Error()
			}
		}
		impl.notifySubscribers(evt, chst)
	}()
}
