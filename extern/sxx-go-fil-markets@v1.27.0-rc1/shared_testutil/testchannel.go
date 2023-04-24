package shared_testutil

import (
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/exp/rand"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
)

// TestChannelParams are params for a new test data transfer channel
type TestChannelParams struct {
	TransferID     datatransfer.TransferID
	BaseCID        cid.Cid
	Selector       datamodel.Node
	SelfPeer       peer.ID
	Sender         peer.ID
	Recipient      peer.ID
	TotalSize      uint64
	IsPull         bool
	Message        string
	Sent           uint64
	Received       uint64
	Queued         uint64
	Status         datatransfer.Status
	Vouchers       []datatransfer.TypedVoucher
	VoucherResults []datatransfer.TypedVoucher
	ReceivedCids   []cid.Cid
}

// TestChannel implements a datatransfer channel with set values
type TestChannel struct {
	selfPeer       peer.ID
	transferID     datatransfer.TransferID
	baseCID        cid.Cid
	selector       datamodel.Node
	sender         peer.ID
	recipient      peer.ID
	totalSize      uint64
	message        string
	isPull         bool
	sent           uint64
	received       uint64
	queued         uint64
	status         datatransfer.Status
	vouchers       []datatransfer.TypedVoucher
	voucherResults []datatransfer.TypedVoucher
	receivedCids   []cid.Cid
}

// NewTestChannel makes a test channel with default params plus non-zero
// values for TestChannelParams
func NewTestChannel(params TestChannelParams) datatransfer.ChannelState {
	peers := GeneratePeers(2)
	tc := &TestChannel{
		selfPeer:   peers[0],
		transferID: datatransfer.TransferID(rand.Uint64()),
		baseCID:    GenerateCids(1)[0],
		selector:   selectorparse.CommonSelector_ExploreAllRecursively,
		sender:     peers[0],
		recipient:  peers[1],
		totalSize:  rand.Uint64(),
		isPull:     params.IsPull,
		status:     params.Status,
		sent:       params.Sent,
		received:   params.Received,
		queued:     params.Queued,
		vouchers: []datatransfer.TypedVoucher{
			{Voucher: basicnode.NewString("Fake DT Voucher"), Type: datatransfer.TypeIdentifier("Fake")},
		},
		voucherResults: []datatransfer.TypedVoucher{
			{Voucher: basicnode.NewString("Fake DT Voucher"), Type: datatransfer.TypeIdentifier("Fake")},
		},
	}

	tc.receivedCids = params.ReceivedCids

	if params.TransferID != 0 {
		tc.transferID = params.TransferID
	}
	if (params.BaseCID != cid.Cid{}) {
		tc.baseCID = params.BaseCID
	}
	if params.Selector != nil {
		tc.selector = params.Selector
	}
	if params.SelfPeer != peer.ID("") {
		tc.selfPeer = params.SelfPeer
	}

	if params.Sender != peer.ID("") {
		tc.sender = params.Sender
	}
	if params.Recipient != peer.ID("") {
		tc.recipient = params.Recipient
	}
	if params.TotalSize != 0 {
		tc.totalSize = params.TotalSize
	}
	if params.Message != "" {
		tc.message = params.Message
	}
	if params.Vouchers != nil {
		tc.vouchers = params.Vouchers
	}
	if params.VoucherResults != nil {
		tc.voucherResults = params.VoucherResults
	}
	return tc
}

func (tc *TestChannel) ReceivedCidsLen() int {
	return len(tc.receivedCids)
}

func (tc *TestChannel) ReceivedCidsTotal() int64 {
	return int64(len(tc.receivedCids))
}

// TransferID returns the transfer id for this channel
func (tc *TestChannel) TransferID() datatransfer.TransferID {
	return tc.transferID
}

// BaseCID returns the CID that is at the root of this data transfer
func (tc *TestChannel) BaseCID() cid.Cid {
	return tc.baseCID
}

// Selector returns the IPLD selector for this data transfer (represented as
// an IPLD node)
func (tc *TestChannel) Selector() datamodel.Node {
	return tc.selector
}

// ReceivedCids returns the cids received so far
func (tc *TestChannel) ReceivedCids() []cid.Cid {
	return tc.receivedCids
}

// TODO actual implementation of those
func (tc *TestChannel) MissingCids() []cid.Cid {
	return nil
}

func (tc *TestChannel) QueuedCidsTotal() int64 {
	return 0
}

func (tc *TestChannel) SentCidsTotal() int64 {
	return 0
}

// Voucher returns the voucher for this data transfer
func (tc *TestChannel) Voucher() datatransfer.TypedVoucher {
	return tc.vouchers[0]
}

// Sender returns the peer id for the node that is sending data
func (tc *TestChannel) Sender() peer.ID {
	return tc.sender
}

// Recipient returns the peer id for the node that is receiving data
func (tc *TestChannel) Recipient() peer.ID {
	return tc.recipient
}

// TotalSize returns the total size for the data being transferred
func (tc *TestChannel) TotalSize() uint64 {
	return tc.totalSize
}

// IsPull returns whether this is a pull request based on who initiated it
func (tc *TestChannel) IsPull() bool {
	return tc.isPull
}

// ChannelID returns the channel id for this channel
func (tc *TestChannel) ChannelID() datatransfer.ChannelID {
	if tc.isPull {
		return datatransfer.ChannelID{ID: tc.transferID, Initiator: tc.recipient, Responder: tc.sender}
	} else {
		return datatransfer.ChannelID{ID: tc.transferID, Initiator: tc.sender, Responder: tc.recipient}
	}
}

// SelfPeer returns the peer this channel belongs to
func (tc *TestChannel) SelfPeer() peer.ID {
	return tc.selfPeer
}

// OtherPeer returns the channel counter party peer
func (tc *TestChannel) OtherPeer() peer.ID {
	if tc.selfPeer == tc.sender {
		return tc.recipient
	}
	return tc.sender
}

// OtherParty returns the opposite party in the channel to the passed in party
func (tc *TestChannel) OtherParty(thisParty peer.ID) peer.ID {
	if tc.sender == thisParty {
		return tc.recipient
	}
	return tc.sender
}
func (tc *TestChannel) BothPaused() bool      { return false }
func (tc *TestChannel) ResponderPaused() bool { return false }
func (tc *TestChannel) InitiatorPaused() bool { return false }
func (tc *TestChannel) SelfPaused() bool      { return false }

// Status is the current status of this channel
func (tc *TestChannel) Status() datatransfer.Status {
	return tc.status
}

// Sent returns the number of bytes sent
func (tc *TestChannel) Sent() uint64 {
	return tc.sent
}

// Received returns the number of bytes received
func (tc *TestChannel) Received() uint64 {
	return tc.received
}

// Received returns the number of bytes received
func (tc *TestChannel) Queued() uint64 {
	return tc.queued
}

// Message offers additional information about the current status
func (tc *TestChannel) Message() string {
	return tc.message
}

// Vouchers returns all vouchers sent on this channel
func (tc *TestChannel) Vouchers() []datatransfer.TypedVoucher {
	return tc.vouchers
}

// VoucherResults are results of vouchers sent on the channel
func (tc *TestChannel) VoucherResults() []datatransfer.TypedVoucher {
	return tc.voucherResults
}

// LastVoucher returns the last voucher sent on the channel
func (tc *TestChannel) LastVoucher() datatransfer.TypedVoucher {
	return tc.vouchers[len(tc.vouchers)-1]
}

// LastVoucherResult returns the last voucher result sent on the channel
func (tc *TestChannel) LastVoucherResult() datatransfer.TypedVoucher {
	return tc.voucherResults[len(tc.voucherResults)-1]
}

func (tc *TestChannel) Stages() *datatransfer.ChannelStages {
	return nil
}

func (tc *TestChannel) DataLimit() uint64 {
	return 0
}

func (tc *TestChannel) RequiresFinalization() bool {
	return false
}
