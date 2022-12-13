package testnodes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

// TestRetrievalClientNode is a node adapter for a retrieval client whose responses
// are stubbed
type TestRetrievalClientNode struct {
	addFundsOnly                      bool // set this to true to test adding funds to an existing payment channel
	payCh                             address.Address
	payChErr                          error
	createPaychMsgCID, addFundsMsgCID cid.Cid
	lane                              uint64
	laneError                         error
	voucher                           *paychtypes.SignedVoucher
	voucherError, waitErr             error
	channelAvailableFunds             retrievalmarket.ChannelAvailableFunds
	checkAvailableFundsErr            error
	fundsAdded                        abi.TokenAmount
	intergrationTest                  bool
	knownAddreses                     map[retrievalmarket.RetrievalPeer][]ma.Multiaddr
	receivedKnownAddresses            map[retrievalmarket.RetrievalPeer]struct{}
	expectedKnownAddresses            map[retrievalmarket.RetrievalPeer]struct{}
	allocateLaneRecorder              func(address.Address)
	createPaymentVoucherRecorder      func(voucher *paychtypes.SignedVoucher)
	getCreatePaymentChannelRecorder   func(address.Address, address.Address, abi.TokenAmount)
}

// TestRetrievalClientNodeParams are parameters for initializing a TestRetrievalClientNode
type TestRetrievalClientNodeParams struct {
	PayCh                       address.Address
	PayChErr                    error
	CreatePaychCID, AddFundsCID cid.Cid
	Lane                        uint64
	LaneError                   error
	Voucher                     *paychtypes.SignedVoucher
	VoucherError                error
	AllocateLaneRecorder        func(address.Address)
	PaymentVoucherRecorder      func(voucher *paychtypes.SignedVoucher)
	PaymentChannelRecorder      func(address.Address, address.Address, abi.TokenAmount)
	AddFundsOnly                bool
	WaitForReadyErr             error
	ChannelAvailableFunds       retrievalmarket.ChannelAvailableFunds
	CheckAvailableFundsErr      error
	IntegrationTest             bool
}

var _ retrievalmarket.RetrievalClientNode = &TestRetrievalClientNode{}

// NewTestRetrievalClientNode initializes a new TestRetrievalClientNode based on the given params
func NewTestRetrievalClientNode(params TestRetrievalClientNodeParams) *TestRetrievalClientNode {
	return &TestRetrievalClientNode{
		addFundsOnly:                    params.AddFundsOnly,
		payCh:                           params.PayCh,
		payChErr:                        params.PayChErr,
		waitErr:                         params.WaitForReadyErr,
		lane:                            params.Lane,
		laneError:                       params.LaneError,
		voucher:                         params.Voucher,
		voucherError:                    params.VoucherError,
		allocateLaneRecorder:            params.AllocateLaneRecorder,
		createPaymentVoucherRecorder:    params.PaymentVoucherRecorder,
		getCreatePaymentChannelRecorder: params.PaymentChannelRecorder,
		createPaychMsgCID:               params.CreatePaychCID,
		addFundsMsgCID:                  params.AddFundsCID,
		channelAvailableFunds:           addZeroesToAvailableFunds(params.ChannelAvailableFunds),
		checkAvailableFundsErr:          params.CheckAvailableFundsErr,
		intergrationTest:                params.IntegrationTest,
		knownAddreses:                   map[retrievalmarket.RetrievalPeer][]ma.Multiaddr{},
		expectedKnownAddresses:          map[retrievalmarket.RetrievalPeer]struct{}{},
		receivedKnownAddresses:          map[retrievalmarket.RetrievalPeer]struct{}{},
	}
}

// GetOrCreatePaymentChannel returns a mocked payment channel
func (trcn *TestRetrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable abi.TokenAmount, tok shared.TipSetToken) (address.Address, cid.Cid, error) {
	if trcn.getCreatePaymentChannelRecorder != nil {
		trcn.getCreatePaymentChannelRecorder(clientAddress, minerAddress, clientFundsAvailable)
	}
	var payCh address.Address
	msgCID := trcn.createPaychMsgCID
	if trcn.addFundsOnly {
		payCh = trcn.payCh
		msgCID = trcn.addFundsMsgCID
	}
	trcn.fundsAdded = clientFundsAvailable
	return payCh, msgCID, trcn.payChErr
}

// AllocateLane creates a mock lane on a payment channel
func (trcn *TestRetrievalClientNode) AllocateLane(ctx context.Context, paymentChannel address.Address) (uint64, error) {
	if trcn.allocateLaneRecorder != nil {
		trcn.allocateLaneRecorder(paymentChannel)
	}
	return trcn.lane, trcn.laneError
}

// CreatePaymentVoucher creates a mock payment voucher based on a channel and lane
func (trcn *TestRetrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount, lane uint64, tok shared.TipSetToken) (*paychtypes.SignedVoucher, error) {
	if trcn.createPaymentVoucherRecorder != nil {
		trcn.createPaymentVoucherRecorder(trcn.voucher)
	}
	if trcn.intergrationTest && amount.GreaterThan(trcn.channelAvailableFunds.ConfirmedAmt) {
		return nil, retrievalmarket.NewShortfallError(big.Sub(amount, trcn.channelAvailableFunds.ConfirmedAmt))
	}
	if trcn.voucherError != nil {
		return nil, trcn.voucherError
	}
	voucher := *trcn.voucher
	voucher.Amount = amount
	return &voucher, nil
}

// GetChainHead returns a mock value for the chain head
func (trcn *TestRetrievalClientNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	return shared.TipSetToken{}, 0, nil
}

// WaitForPaymentChannelReady simulates waiting for a payment channel to finish adding funds
func (trcn *TestRetrievalClientNode) WaitForPaymentChannelReady(ctx context.Context, messageCID cid.Cid) (address.Address, error) {
	if messageCID.Equals(trcn.createPaychMsgCID) && !trcn.addFundsOnly {
		if trcn.intergrationTest {
			trcn.channelAvailableFunds.ConfirmedAmt = big.Add(trcn.channelAvailableFunds.ConfirmedAmt, trcn.fundsAdded)
		}
		return trcn.payCh, trcn.waitErr
	}
	if messageCID.Equals(trcn.addFundsMsgCID) && trcn.addFundsOnly {
		if trcn.intergrationTest {
			trcn.channelAvailableFunds.ConfirmedAmt = big.Add(trcn.channelAvailableFunds.ConfirmedAmt, trcn.fundsAdded)
		}
		return trcn.payCh, trcn.waitErr
	}
	if trcn.channelAvailableFunds.PendingWaitSentinel != nil && messageCID.Equals(*trcn.channelAvailableFunds.PendingWaitSentinel) {
		if trcn.intergrationTest {
			trcn.channelAvailableFunds.ConfirmedAmt = big.Add(trcn.channelAvailableFunds.ConfirmedAmt, trcn.channelAvailableFunds.PendingAmt)
			trcn.channelAvailableFunds.PendingAmt = trcn.channelAvailableFunds.QueuedAmt
			trcn.channelAvailableFunds.PendingWaitSentinel = &shared_testutil.GenerateCids(1)[0]
		}
		return trcn.payCh, trcn.waitErr
	}
	return address.Undef, fmt.Errorf("expected messageCID: %s does not match actual: %s", trcn.addFundsMsgCID, messageCID)
}

// ExpectKnownAddresses stubs a return for a look up of known addresses for the given retrieval peer
// and the fact that it was looked up is verified with VerifyExpectations
func (trcn *TestRetrievalClientNode) ExpectKnownAddresses(p retrievalmarket.RetrievalPeer, maddrs []ma.Multiaddr) {
	trcn.expectedKnownAddresses[p] = struct{}{}
	trcn.knownAddreses[p] = maddrs
}

// GetKnownAddresses gets any on known multiaddrs for a given address, so we can add to the peer store
func (trcn *TestRetrievalClientNode) GetKnownAddresses(ctx context.Context, p retrievalmarket.RetrievalPeer, tok shared.TipSetToken) ([]ma.Multiaddr, error) {
	trcn.receivedKnownAddresses[p] = struct{}{}
	addrs, ok := trcn.knownAddreses[p]
	if !ok {
		return nil, errors.New("Provider not found")
	}
	return addrs, nil
}

// ResetChannelAvailableFunds is a way to manually change the funds in the payment channel
func (trcn *TestRetrievalClientNode) ResetChannelAvailableFunds(channelAvailableFunds retrievalmarket.ChannelAvailableFunds) {
	trcn.channelAvailableFunds = addZeroesToAvailableFunds(channelAvailableFunds)
}

// VerifyExpectations verifies that all expected known addresses were looked up
func (trcn *TestRetrievalClientNode) VerifyExpectations(t *testing.T) {
	require.Equal(t, trcn.expectedKnownAddresses, trcn.receivedKnownAddresses)
}

// CheckAvailableFunds returns the amount of available funds in a payment channel
func (trcn *TestRetrievalClientNode) CheckAvailableFunds(ctx context.Context, payCh address.Address) (retrievalmarket.ChannelAvailableFunds, error) {
	return trcn.channelAvailableFunds, trcn.checkAvailableFundsErr
}

func addZeroesToAvailableFunds(channelAvailableFunds retrievalmarket.ChannelAvailableFunds) retrievalmarket.ChannelAvailableFunds {
	if channelAvailableFunds.ConfirmedAmt.Nil() {
		channelAvailableFunds.ConfirmedAmt = big.Zero()
	}
	if channelAvailableFunds.PendingAmt.Nil() {
		channelAvailableFunds.PendingAmt = big.Zero()
	}
	if channelAvailableFunds.QueuedAmt.Nil() {
		channelAvailableFunds.QueuedAmt = big.Zero()
	}
	if channelAvailableFunds.VoucherReedeemedAmt.Nil() {
		channelAvailableFunds.VoucherReedeemedAmt = big.Zero()
	}
	return channelAvailableFunds
}
