package requestvalidation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestValidatePush(t *testing.T) {
	fve := &fakeValidationEnvironment{}
	sender := shared_testutil.GeneratePeers(1)[0]
	testDp := shared_testutil.MakeTestDealProposal()
	voucher := rm.BindnodeRegistry.TypeToNode(testDp)
	requestValidator := requestvalidation.NewProviderRequestValidator(fve)
	validationResult, err := requestValidator.ValidatePush(datatransfer.ChannelID{}, sender, voucher, testDp.PayloadCID, selectorparse.CommonSelector_ExploreAllRecursively)
	require.Nil(t, validationResult.VoucherResult)
	require.Error(t, err)
}

func dealResponseToVoucher(t *testing.T, status rm.DealStatus, id rm.DealID, message string, owed *abi.TokenAmount) *datatransfer.TypedVoucher {
	dr := rm.DealResponse{
		Status:  status,
		ID:      id,
		Message: message,
	}
	if owed != nil {
		dr.PaymentOwed = *owed
	}
	node := rm.BindnodeRegistry.TypeToNode(&dr)
	return &datatransfer.TypedVoucher{Voucher: node, Type: rm.DealResponseType}
}

func TestValidatePull(t *testing.T) {
	proposal := shared_testutil.MakeTestDealProposal()
	node := rm.BindnodeRegistry.TypeToNode(proposal)
	proposalVoucher := datatransfer.TypedVoucher{Voucher: node, Type: rm.DealProposalType}
	zero := big.Zero()

	testCases := map[string]struct {
		fve                        fakeValidationEnvironment
		sender                     peer.ID
		voucher                    datatransfer.TypedVoucher
		baseCid                    cid.Cid
		selector                   datamodel.Node
		expectedVoucherResult      *datatransfer.TypedVoucher
		expectedError              error
		expectAccepted             bool
		expectForcePause           bool
		expectDataLimit            uint64
		expectRequiresFinalization bool
	}{
		"not a retrieval voucher": {
			expectedError: errors.New("empty voucher"),
		},
		"proposal and base cid do not match": {
			baseCid:               shared_testutil.GenerateCids(1)[0],
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusRejected, proposal.ID, "incorrect CID for this proposal", nil),
		},
		"proposal and selector do not match": {
			baseCid:               proposal.PayloadCID,
			selector:              builder.NewSelectorSpecBuilder(basicnode.Prototype.Any).Matcher().Node(),
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusRejected, proposal.ID, "incorrect selector specified for this proposal", nil),
		},
		"get piece other err": {
			fve: fakeValidationEnvironment{
				RunDealDecisioningLogicAccepted: true,
				GetPieceErr:                     errors.New("something went wrong"),
			},
			baseCid:               proposal.PayloadCID,
			selector:              selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusErrored, proposal.ID, "something went wrong", nil),
		},
		"get piece not found err": {
			fve: fakeValidationEnvironment{
				RunDealDecisioningLogicAccepted: true,
				GetPieceErr:                     rm.ErrNotFound,
			},
			baseCid:               proposal.PayloadCID,
			selector:              selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusDealNotFound, proposal.ID, rm.ErrNotFound.Error(), nil),
		},
		"check deal params err": {
			fve: fakeValidationEnvironment{
				CheckDealParamsError: errors.New("something went wrong"),
			},
			baseCid:               proposal.PayloadCID,
			selector:              selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusRejected, proposal.ID, "something went wrong", nil),
		},
		"run deal decioning error": {
			fve: fakeValidationEnvironment{
				RunDealDecisioningLogicError: errors.New("something went wrong"),
			},
			baseCid:               proposal.PayloadCID,
			selector:              selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusErrored, proposal.ID, "something went wrong", nil),
		},
		"run deal decioning rejected": {
			fve: fakeValidationEnvironment{
				RunDealDecisioningLogicFailReason: "something went wrong",
			},
			baseCid:               proposal.PayloadCID,
			selector:              selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:               proposalVoucher,
			expectedVoucherResult: dealResponseToVoucher(t, rm.DealStatusRejected, proposal.ID, "something went wrong", nil),
		},
		"begin tracking error": {
			fve: fakeValidationEnvironment{
				BeginTrackingError:              errors.New("everything is awful"),
				RunDealDecisioningLogicAccepted: true,
			},
			baseCid:       proposal.PayloadCID,
			selector:      selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:       proposalVoucher,
			expectedError: errors.New("everything is awful"),
		},
		"success": {
			fve: fakeValidationEnvironment{
				RunDealDecisioningLogicAccepted: true,
			},
			baseCid:                    proposal.PayloadCID,
			selector:                   selectorparse.CommonSelector_ExploreAllRecursively,
			voucher:                    proposalVoucher,
			expectedVoucherResult:      dealResponseToVoucher(t, rm.DealStatusAccepted, proposal.ID, "", &zero),
			expectAccepted:             true,
			expectForcePause:           true,
			expectDataLimit:            proposal.PaymentInterval,
			expectRequiresFinalization: true,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			requestValidator := requestvalidation.NewProviderRequestValidator(&data.fve)
			validationResult, err := requestValidator.ValidatePull(datatransfer.ChannelID{}, data.sender, data.voucher.Voucher, data.baseCid, data.selector)
			if data.expectedVoucherResult == nil {
				require.Nil(t, validationResult.VoucherResult)
			} else {
				require.True(t, data.expectedVoucherResult.Equals(*validationResult.VoucherResult))
			}
			require.Equal(t, data.expectAccepted, validationResult.Accepted)
			require.Equal(t, data.expectForcePause, validationResult.ForcePause)
			require.Equal(t, data.expectDataLimit, validationResult.DataLimit)
			require.Equal(t, data.expectRequiresFinalization, validationResult.RequiresFinalization)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, data.expectedError.Error())
			}
		})
	}
}

func TestValidateRestart(t *testing.T) {

	dealID := rm.DealID(10)
	defaultCurrentInterval := uint64(1000)
	defaultIntervalIncrease := uint64(500)
	defaultPricePerByte := abi.NewTokenAmount(500)
	defaultPaymentPerInterval := big.Mul(defaultPricePerByte, abi.NewTokenAmount(int64(defaultCurrentInterval)))
	defaultUnsealPrice := defaultPaymentPerInterval
	params, err := rm.NewParamsV1(
		defaultPricePerByte,
		defaultCurrentInterval,
		defaultIntervalIncrease,
		selectorparse.CommonSelector_ExploreAllRecursively,
		nil,
		defaultUnsealPrice,
	)
	require.NoError(t, err)
	proposal := rm.DealProposal{
		ID:     dealID,
		Params: params,
	}
	node := rm.BindnodeRegistry.TypeToNode(&proposal)
	proposalVoucher := datatransfer.TypedVoucher{Voucher: node, Type: rm.DealProposalType}

	testCases := map[string]struct {
		status                rm.DealStatus
		fundReceived          abi.TokenAmount
		voucher               datatransfer.TypedVoucher
		queued                uint64
		dtStatus              datatransfer.Status
		dealErr               error
		expectedValidation    datatransfer.ValidationResult
		expectedValidationErr error
	}{
		"normal operation": {
			status:       rm.DealStatusOngoing,
			fundReceived: big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			voucher:      proposalVoucher,
			queued:       defaultCurrentInterval,
			dtStatus:     datatransfer.Ongoing,
			expectedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"unsealing": {
			status:       rm.DealStatusUnsealing,
			fundReceived: defaultUnsealPrice,
			voucher:      proposalVoucher,
			queued:       0,
			dtStatus:     datatransfer.ResponderPaused,
			expectedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				ForcePause:           true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"last payment, no money owed": {
			status:       rm.DealStatusFinalizing,
			fundReceived: big.Add(defaultUnsealPrice, big.Add(defaultPaymentPerInterval, defaultPaymentPerInterval)),
			voucher:      proposalVoucher,
			queued:       defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:     datatransfer.Finalizing,
			expectedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: false,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"last payment, money owed": {
			status:       rm.DealStatusFundsNeededLastPayment,
			fundReceived: big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			voucher:      proposalVoucher,
			queued:       defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:     datatransfer.Finalizing,
			expectedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"get deal error": {
			voucher: proposalVoucher,
			dealErr: errors.New("something went wrong"),
			expectedValidation: datatransfer.ValidationResult{
				Accepted:      false,
				VoucherResult: dealResponseToVoucher(t, rm.DealStatusErrored, dealID, "something went wrong", nil),
			},
		},
		"wrong voucher type": {
			voucher:               datatransfer.TypedVoucher{Voucher: basicnode.NewString("bad voucher"), Type: datatransfer.TypeIdentifier("bad")},
			expectedValidationErr: errors.New("wrong voucher type"),
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			dealState := &rm.ProviderDealState{
				Status:        data.status,
				FundsReceived: data.fundReceived,
				DealProposal: rm.DealProposal{
					ID:     dealID,
					Params: params,
				},
			}
			fve := &fakeValidationEnvironment{GetDeal: *dealState, GetError: data.dealErr}
			requestValidator := requestvalidation.NewProviderRequestValidator(fve)
			chst := shared_testutil.NewTestChannel(shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{
					data.voucher,
				},
				Status: data.dtStatus,
				Queued: data.queued,
			})
			validationResult, err := requestValidator.ValidateRestart(datatransfer.ChannelID{}, chst)
			require.True(t, data.expectedValidation.Equals(validationResult))
			if data.expectedValidationErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, data.expectedValidationErr.Error())
			}
		})
	}
}

type fakeValidationEnvironment struct {
	IsUnsealedPiece                   bool
	PieceInfo                         piecestore.PieceInfo
	GetPieceErr                       error
	CheckDealParamsError              error
	RunDealDecisioningLogicAccepted   bool
	RunDealDecisioningLogicFailReason string
	RunDealDecisioningLogicError      error
	BeginTrackingError                error

	Ask      rm.Ask
	GetDeal  rm.ProviderDealState
	GetError error
}

func (fve *fakeValidationEnvironment) GetAsk(ctx context.Context, payloadCid cid.Cid, pieceCid *cid.Cid,
	piece piecestore.PieceInfo, isUnsealed bool, client peer.ID) (rm.Ask, error) {
	return fve.Ask, nil
}

func (fve *fakeValidationEnvironment) GetPiece(c cid.Cid, pieceCID *cid.Cid) (piecestore.PieceInfo, bool, error) {
	return fve.PieceInfo, fve.IsUnsealedPiece, fve.GetPieceErr
}

// CheckDealParams verifies the given deal params are acceptable
func (fve *fakeValidationEnvironment) CheckDealParams(ask rm.Ask, pricePerByte abi.TokenAmount, paymentInterval uint64, paymentIntervalIncrease uint64, unsealPrice abi.TokenAmount) error {
	return fve.CheckDealParamsError
}

// RunDealDecisioningLogic runs custom deal decision logic to decide if a deal is accepted, if present
func (fve *fakeValidationEnvironment) RunDealDecisioningLogic(ctx context.Context, state rm.ProviderDealState) (bool, string, error) {
	return fve.RunDealDecisioningLogicAccepted, fve.RunDealDecisioningLogicFailReason, fve.RunDealDecisioningLogicError
}

// StateMachines returns the FSM Group to begin tracking with
func (fve *fakeValidationEnvironment) BeginTracking(pds rm.ProviderDealState) error {
	return fve.BeginTrackingError
}

func (fve *fakeValidationEnvironment) Get(dealID rm.ProviderDealIdentifier) (rm.ProviderDealState, error) {
	return fve.GetDeal, fve.GetError
}
