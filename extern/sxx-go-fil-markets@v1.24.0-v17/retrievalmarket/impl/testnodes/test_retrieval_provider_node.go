package testnodes

import (
	"bytes"
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("retrieval_provnode_test")

type expectedVoucherKey struct {
	paymentChannel string
	voucher        string
	proof          string
	expectedAmount string
}

type voucherResult struct {
	amount abi.TokenAmount
	err    error
}

// TestRetrievalProviderNode is a node adapter for a retrieval provider whose
// responses are mocked
type TestRetrievalProviderNode struct {
	ChainHeadError   error
	lk               sync.Mutex
	expectedVouchers map[expectedVoucherKey]voucherResult

	expectedPricingParamDeals []abi.DealID
	receivedPricingParamDeals []abi.DealID

	expectedPricingPieceCID cid.Cid
	receivedPricingPieceCID cid.Cid

	isVerified       bool
	receivedVouchers []abi.TokenAmount
	unsealPaused     chan struct{}
}

var _ retrievalmarket.RetrievalProviderNode = &TestRetrievalProviderNode{}

// NewTestRetrievalProviderNode instantiates a new TestRetrievalProviderNode
func NewTestRetrievalProviderNode() *TestRetrievalProviderNode {
	return &TestRetrievalProviderNode{
		expectedVouchers: make(map[expectedVoucherKey]voucherResult),
	}
}

func (trpn *TestRetrievalProviderNode) MarkVerified() {
	trpn.isVerified = true
}

func (trpn *TestRetrievalProviderNode) ExpectPricingParams(pieceCID cid.Cid, deals []abi.DealID) {
	trpn.expectedPricingPieceCID = pieceCID
	trpn.expectedPricingParamDeals = deals
}

func (trpn *TestRetrievalProviderNode) GetRetrievalPricingInput(_ context.Context, pieceCID cid.Cid, deals []abi.DealID) (retrievalmarket.PricingInput, error) {
	trpn.receivedPricingParamDeals = deals
	trpn.receivedPricingPieceCID = pieceCID

	return retrievalmarket.PricingInput{
		VerifiedDeal: trpn.isVerified,
	}, nil
}

// VerifyExpectations verifies that all expected calls were made and no other calls
// were made
func (trpn *TestRetrievalProviderNode) VerifyExpectations(t *testing.T) {
	require.Equal(t, len(trpn.expectedVouchers), len(trpn.receivedVouchers))
	require.Equal(t, trpn.expectedPricingPieceCID, trpn.receivedPricingPieceCID)
	require.Equal(t, trpn.expectedPricingParamDeals, trpn.receivedPricingParamDeals)
}

// SavePaymentVoucher simulates saving a payment voucher with a stubbed result
func (trpn *TestRetrievalProviderNode) SavePaymentVoucher(
	ctx context.Context,
	paymentChannel address.Address,
	voucher *paychtypes.SignedVoucher,
	proof []byte,
	expectedAmount abi.TokenAmount,
	tok shared.TipSetToken) (abi.TokenAmount, error) {

	trpn.lk.Lock()
	defer trpn.lk.Unlock()

	key, err := trpn.toExpectedVoucherKey(paymentChannel, voucher, proof, voucher.Amount)
	if err != nil {
		return abi.TokenAmount{}, err
	}

	max := big.Zero()
	for _, amt := range trpn.receivedVouchers {
		max = big.Max(max, amt)
	}
	trpn.receivedVouchers = append(trpn.receivedVouchers, voucher.Amount)
	rcvd := big.Sub(voucher.Amount, max)
	if rcvd.LessThan(big.Zero()) {
		rcvd = big.Zero()
	}
	if len(trpn.expectedVouchers) == 0 {
		return rcvd, nil
	}

	result, ok := trpn.expectedVouchers[key]
	if ok {
		return rcvd, result.err
	}
	var amts []abi.TokenAmount
	for _, vchr := range trpn.expectedVouchers {
		amts = append(amts, vchr.amount)
	}
	sort.Slice(amts, func(i, j int) bool {
		return amts[i].LessThan(amts[j])
	})
	err = xerrors.Errorf("SavePaymentVoucher failed - voucher %d didnt match expected voucher %d in %s", voucher.Amount, expectedAmount, amts)
	return abi.TokenAmount{}, err
}

// GetMinerWorkerAddress translates an address
func (trpn *TestRetrievalProviderNode) GetMinerWorkerAddress(ctx context.Context, addr address.Address, tok shared.TipSetToken) (address.Address, error) {
	return addr, nil
}

// GetChainHead returns a mock value for the chain head
func (trpn *TestRetrievalProviderNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	return []byte{42}, 0, trpn.ChainHeadError
}

// --- Non-interface Functions

// to ExpectedVoucherKey creates a lookup key for expected vouchers.
func (trpn *TestRetrievalProviderNode) toExpectedVoucherKey(paymentChannel address.Address, voucher *paychtypes.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount) (expectedVoucherKey, error) {
	pcString := paymentChannel.String()
	buf := new(bytes.Buffer)
	if err := voucher.MarshalCBOR(buf); err != nil {
		return expectedVoucherKey{}, err
	}
	voucherString := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	proofString := string(proof)
	expectedAmountString := expectedAmount.String()
	return expectedVoucherKey{pcString, voucherString, proofString, expectedAmountString}, nil
}

// ExpectVoucher sets a voucher to be expected by SavePaymentVoucher
//     paymentChannel: the address of the payment channel the client creates
//     voucher: the voucher to match
//     proof: the proof to use (can be blank)
// 	   expectedAmount: the expected tokenamount for this voucher
//     actualAmount: the actual amount to use.  use same as expectedAmount unless you want to trigger an error
//     expectedErr:  an error message to expect
func (trpn *TestRetrievalProviderNode) ExpectVoucher(
	paymentChannel address.Address,
	voucher *paychtypes.SignedVoucher,
	proof []byte,
	expectedAmount abi.TokenAmount,
	actualAmount abi.TokenAmount, // the actual amount it should have (same unless you want to trigger an error)
	expectedErr error) error {
	vch := *voucher
	vch.Amount = expectedAmount
	key, err := trpn.toExpectedVoucherKey(paymentChannel, &vch, proof, expectedAmount)
	if err != nil {
		return err
	}
	trpn.expectedVouchers[key] = voucherResult{actualAmount, expectedErr}
	return nil
}

func (trpn *TestRetrievalProviderNode) AddReceivedVoucher(amt abi.TokenAmount) {
	trpn.receivedVouchers = append(trpn.receivedVouchers, amt)
}

func (trpn *TestRetrievalProviderNode) MaxReceivedVoucher() abi.TokenAmount {
	max := abi.NewTokenAmount(0)
	for _, amt := range trpn.receivedVouchers {
		max = big.Max(max, amt)
	}
	return max
}
