package providerstates_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/paych"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmtesting "github.com/filecoin-project/go-fil-markets/retrievalmarket/testing"
	"github.com/filecoin-project/go-fil-markets/shared"
	testnet "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestUnsealData(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runUnsealData := func(t *testing.T,
		node *testnodes.TestRetrievalProviderNode,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		err := providerstates.UnsealData(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	expectedPiece := testnet.GenerateCids(1)[0]
	proposal := rm.DealProposal{
		ID:         rm.DealID(10),
		PayloadCID: expectedPiece,
		Params:     rm.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
	}

	pieceCid := testnet.GenerateCids(1)[0]

	sectorID := abi.SectorNumber(rand.Uint64())
	offset := abi.PaddedPieceSize(rand.Uint64())
	length := abi.PaddedPieceSize(rand.Uint64())

	sectorID2 := abi.SectorNumber(rand.Uint64())
	offset2 := abi.PaddedPieceSize(rand.Uint64())
	length2 := abi.PaddedPieceSize(rand.Uint64())

	makeDeals := func() *rm.ProviderDealState {
		return &rm.ProviderDealState{
			DealProposal: proposal,
			Status:       rm.DealStatusUnsealing,
			PieceInfo: &piecestore.PieceInfo{
				PieceCID: pieceCid,
				Deals: []piecestore.DealInfo{
					{
						DealID:   abi.DealID(rand.Uint64()),
						SectorID: sectorID,
						Offset:   offset,
						Length:   length,
					},
					{
						DealID:   abi.DealID(rand.Uint64()),
						SectorID: sectorID2,
						Offset:   offset2,
						Length:   length2,
					},
				},
			},
			FundsReceived: abi.NewTokenAmount(0),
		}
	}

	t.Run("unseals successfully", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := makeDeals()
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runUnsealData(t, node, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusUnsealed)
	})

	t.Run("PrepareBlockstore error", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := makeDeals()
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.PrepareBlockstoreError = errors.New("Something went wrong")
		}
		runUnsealData(t, node, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusFailing)
		require.Equal(t, dealState.Message, "Something went wrong")
	})
}

func TestUnpauseDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runUnpauseDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		dealState.ChannelID = &datatransfer.ChannelID{
			Initiator: "initiator",
			Responder: dealState.Receiver,
			ID:        1,
		}
		err := providerstates.UnpauseDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusUnsealed)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runUnpauseDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusUnsealed)
	})
	t.Run("error resuming channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusUnsealed)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.ResumeDataTransferError = errors.New("something went wrong resuming")
		}
		runUnpauseDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong resuming")
	})
}

func TestUpdateFunding(t *testing.T) {
	ctx := context.Background()
	emptyDealPayment := rm.DealPayment{}
	emptyDealPaymentNode := rm.BindnodeRegistry.TypeToNode(&emptyDealPayment)
	emptyDealPaymentVoucher := datatransfer.TypedVoucher{Voucher: emptyDealPaymentNode, Type: rm.DealPaymentType}
	emptyDealProposal := rm.DealProposal{}
	emptyDealProposalNode := rm.BindnodeRegistry.TypeToNode(&emptyDealProposal)
	emptyDealProposalVoucher := datatransfer.TypedVoucher{Voucher: emptyDealProposalNode, Type: rm.DealProposalType}
	dealResponseVoucher := func(resp rm.DealResponse) *datatransfer.TypedVoucher {
		node := rm.BindnodeRegistry.TypeToNode(&resp)
		return &datatransfer.TypedVoucher{Voucher: node, Type: rm.DealResponseType}
	}
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	testCases := map[string]struct {
		status                     rm.DealStatus
		emptyChannelID             bool
		fundReceived               abi.TokenAmount
		lastVoucher                datatransfer.TypedVoucher
		queued                     uint64
		dtStatus                   datatransfer.Status
		channelStateErr            error
		chainHeadErr               error
		savePaymentErr             error
		savePaymentAmount          abi.TokenAmount
		updateValidationStatusErr  error
		expectedFinalStatus        rm.DealStatus
		expectedFundsReceived      abi.TokenAmount
		expectedReceivedValidation datatransfer.ValidationResult
		expectedMessage            string
	}{
		"when empty channel id does nothing": {
			status:              rm.DealStatusFundsNeededUnseal,
			emptyChannelID:      true,
			expectedFinalStatus: rm.DealStatusFundsNeededUnseal,
		},
		"when error getting channel state, errors channel": {
			status:              rm.DealStatusFundsNeededUnseal,
			channelStateErr:     errors.New("couldn't get channel state"),
			expectedFinalStatus: rm.DealStatusErrored,
			expectedMessage:     "couldn't get channel state",
		},
		"when last voucher incorrect type, sends response": {
			status:              rm.DealStatusFundsNeededUnseal,
			lastVoucher:         datatransfer.TypedVoucher{Voucher: basicnode.NewString("Fake Voucher"), Type: datatransfer.TypeIdentifier("Fake")},
			expectedFinalStatus: rm.DealStatusFundsNeededUnseal,
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: false,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:      dealID,
					Message: "wrong voucher type",
					Status:  rm.DealStatusErrored,
				}),
			},
		},
		"when received payment with nothing owed on unseal, updates status, forces pause": {
			status:                rm.DealStatusFundsNeededUnseal,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                0,
			dtStatus:              datatransfer.ResponderPaused,
			savePaymentAmount:     defaultUnsealPrice,
			expectedFinalStatus:   rm.DealStatusUnsealing,
			expectedFundsReceived: defaultUnsealPrice,
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				ForcePause:           true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"when received payment with nothing owed, updates data limits, funds, and status": {
			status:                rm.DealStatusFundsNeeded,
			fundReceived:          defaultUnsealPrice,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval,
			dtStatus:              datatransfer.ResponderPaused,
			savePaymentAmount:     defaultPaymentPerInterval,
			expectedFinalStatus:   rm.DealStatusOngoing,
			expectedFundsReceived: big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"when received payment with nothing owed on last payment, sends response, sets finalization false, updates status": {
			status:                rm.DealStatusFundsNeededLastPayment,
			fundReceived:          big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:              datatransfer.Finalizing,
			savePaymentAmount:     defaultPaymentPerInterval,
			expectedFinalStatus:   rm.DealStatusFinalizing,
			expectedFundsReceived: big.Add(defaultUnsealPrice, big.Add(defaultPaymentPerInterval, defaultPaymentPerInterval)),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:     dealID,
					Status: rm.DealStatusCompleted,
				}),
				RequiresFinalization: false,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"when received payment with more owed on unseal, sends response, stays paused": {
			status:                rm.DealStatusFundsNeededUnseal,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                0,
			dtStatus:              datatransfer.ResponderPaused,
			savePaymentAmount:     big.Div(defaultUnsealPrice, big.NewInt(2)),
			expectedFinalStatus:   rm.DealStatusFundsNeededUnseal,
			expectedFundsReceived: big.Div(defaultUnsealPrice, big.NewInt(2)),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:          dealID,
					Status:      rm.DealStatusFundsNeededUnseal,
					PaymentOwed: big.Div(defaultUnsealPrice, big.NewInt(2)),
				}),
				RequiresFinalization: true,
				ForcePause:           true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"when received payment with more owed, sends response and does not change data limit": {
			status:                rm.DealStatusFundsNeeded,
			fundReceived:          defaultUnsealPrice,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval,
			dtStatus:              datatransfer.ResponderPaused,
			savePaymentAmount:     big.Div(defaultPaymentPerInterval, big.NewInt(2)),
			expectedFinalStatus:   rm.DealStatusFundsNeeded,
			expectedFundsReceived: big.Add(defaultUnsealPrice, big.Div(defaultPaymentPerInterval, big.NewInt(2))),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:          dealID,
					Status:      rm.DealStatusFundsNeeded,
					PaymentOwed: big.Div(defaultPaymentPerInterval, big.NewInt(2)),
				}),
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"when received payment with more owed on last payment, sends response and does not change finalization status": {
			status:                rm.DealStatusFundsNeededLastPayment,
			fundReceived:          big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:              datatransfer.Finalizing,
			savePaymentAmount:     big.Div(defaultPaymentPerInterval, big.NewInt(2)),
			expectedFinalStatus:   rm.DealStatusFundsNeededLastPayment,
			expectedFundsReceived: big.Add(defaultUnsealPrice, big.Add(defaultPaymentPerInterval, big.Div(defaultPaymentPerInterval, big.NewInt(2)))),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:          dealID,
					Status:      rm.DealStatusFundsNeededLastPayment,
					PaymentOwed: big.Div(defaultPaymentPerInterval, big.NewInt(2)),
				}),
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"when money owed with no payment on unseal, leaves request paused": {
			status:              rm.DealStatusFundsNeededUnseal,
			lastVoucher:         emptyDealProposalVoucher,
			queued:              0,
			dtStatus:            datatransfer.ResponderPaused,
			expectedFinalStatus: rm.DealStatusFundsNeededUnseal,
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				ForcePause:           true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"when money owed with no payment, sends response and does not change data limit": {
			status:                rm.DealStatusFundsNeeded,
			fundReceived:          defaultUnsealPrice,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval,
			dtStatus:              datatransfer.ResponderPaused,
			expectedFinalStatus:   rm.DealStatusFundsNeeded,
			expectedFundsReceived: defaultUnsealPrice,
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:          dealID,
					Status:      rm.DealStatusFundsNeeded,
					PaymentOwed: defaultPaymentPerInterval,
				}),
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval,
			},
		},
		"when no payment and money owed for last payment, sends response and does not change finalization status": {
			status:                rm.DealStatusFundsNeededLastPayment,
			fundReceived:          big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:              datatransfer.Finalizing,
			expectedFinalStatus:   rm.DealStatusFundsNeededLastPayment,
			expectedFundsReceived: big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:          dealID,
					Status:      rm.DealStatusFundsNeededLastPayment,
					PaymentOwed: defaultPaymentPerInterval,
				}),
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"when surplus payment with nothing owed, updates data limits accordingly": {
			status:                rm.DealStatusFundsNeeded,
			fundReceived:          defaultUnsealPrice,
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval,
			dtStatus:              datatransfer.ResponderPaused,
			savePaymentAmount:     big.Mul(defaultPricePerByte, big.NewIntUnsigned(defaultCurrentInterval+defaultCurrentInterval+defaultIntervalIncrease)),
			expectedFinalStatus:   rm.DealStatusOngoing,
			expectedFundsReceived: big.Add(defaultUnsealPrice, big.Mul(defaultPricePerByte, big.NewIntUnsigned(defaultCurrentInterval+defaultCurrentInterval+defaultIntervalIncrease))),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit: defaultCurrentInterval +
					defaultCurrentInterval + defaultIntervalIncrease +
					defaultCurrentInterval + (2 * defaultIntervalIncrease),
			},
		},
		"when no money owed and no payment, updates data limits and status": {
			status:                rm.DealStatusFundsNeeded,
			fundReceived:          big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval,
			dtStatus:              datatransfer.ResponderPaused,
			expectedFinalStatus:   rm.DealStatusOngoing,
			expectedFundsReceived: big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"when no money owed and no payment on last payment, sends response, sets finalization false, updates status": {
			status:                rm.DealStatusFundsNeededLastPayment,
			fundReceived:          big.Add(defaultUnsealPrice, big.Add(defaultPaymentPerInterval, defaultPaymentPerInterval)),
			lastVoucher:           emptyDealPaymentVoucher,
			queued:                defaultCurrentInterval + defaultCurrentInterval,
			dtStatus:              datatransfer.Finalizing,
			expectedFinalStatus:   rm.DealStatusFinalizing,
			expectedFundsReceived: big.Add(defaultUnsealPrice, big.Add(defaultPaymentPerInterval, defaultPaymentPerInterval)),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: true,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:     dealID,
					Status: rm.DealStatusCompleted,
				}),
				RequiresFinalization: false,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
		},
		"get chain head error": {
			status:              rm.DealStatusFundsNeededUnseal,
			lastVoucher:         emptyDealPaymentVoucher,
			chainHeadErr:        errors.New("something went wrong"),
			expectedFinalStatus: rm.DealStatusFailing,
			expectedMessage:     "something went wrong",
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: false,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:      dealID,
					Message: "something went wrong",
					Status:  rm.DealStatusErrored,
				}),
			},
		},
		"save payment voucher error": {
			status:              rm.DealStatusFundsNeededUnseal,
			lastVoucher:         emptyDealPaymentVoucher,
			savePaymentErr:      errors.New("something went wrong"),
			expectedFinalStatus: rm.DealStatusFailing,
			expectedMessage:     "something went wrong",
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted: false,
				VoucherResult: dealResponseVoucher(rm.DealResponse{
					ID:      dealID,
					Message: "something went wrong",
					Status:  rm.DealStatusErrored,
				}),
			},
		},
		"update validation status error": {
			status:                    rm.DealStatusFundsNeeded,
			fundReceived:              big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			lastVoucher:               emptyDealPaymentVoucher,
			queued:                    defaultCurrentInterval,
			dtStatus:                  datatransfer.ResponderPaused,
			updateValidationStatusErr: errors.New("something went wrong"),
			expectedFinalStatus:       rm.DealStatusErrored,
			expectedFundsReceived:     big.Add(defaultUnsealPrice, defaultPaymentPerInterval),
			expectedReceivedValidation: datatransfer.ValidationResult{
				Accepted:             true,
				RequiresFinalization: true,
				DataLimit:            defaultCurrentInterval + defaultCurrentInterval + defaultIntervalIncrease,
			},
			expectedMessage: "something went wrong",
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			if data.fundReceived.Nil() {
				data.fundReceived = big.Zero()
			}
			if data.expectedFundsReceived.Nil() {
				data.expectedFundsReceived = big.Zero()
			}
			params, err := rm.NewParamsV1(
				defaultPricePerByte,
				defaultCurrentInterval,
				defaultIntervalIncrease,
				selectorparse.CommonSelector_ExploreAllRecursively,
				nil,
				defaultUnsealPrice,
			)
			require.NoError(t, err)
			dealState := &rm.ProviderDealState{
				Status:        data.status,
				FundsReceived: data.fundReceived,
				DealProposal: rm.DealProposal{
					ID:     dealID,
					Params: params,
				},
			}
			if !data.emptyChannelID {
				dealState.ChannelID = &datatransfer.ChannelID{
					Initiator: "initiator",
					Responder: dealState.Receiver,
					ID:        1,
				}
			}
			chst := testnet.NewTestChannel(testnet.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{
					data.lastVoucher,
				},
				Status: data.dtStatus,
				Queued: data.queued,
			})
			node := &simpleProviderNode{
				chainHeadErr:   data.chainHeadErr,
				receivedAmount: data.savePaymentAmount,
				receivedErr:    data.savePaymentErr,
			}
			environment := rmtesting.NewTestProviderDealEnvironment(node)
			environment.ChannelStateError = data.channelStateErr
			environment.ReturnedChannelState = chst
			environment.UpdateValidationStatusError = data.updateValidationStatusErr
			fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
			err = providerstates.UpdateFunding(fsmCtx, environment, *dealState)
			require.NoError(t, err)
			fsmCtx.ReplayEvents(t, dealState)
			require.Equal(t, data.expectedFinalStatus, dealState.Status)
			require.True(t, data.expectedFundsReceived.Equals(dealState.FundsReceived))
			require.Equal(t, data.expectedMessage, dealState.Message)
			require.True(t, data.expectedReceivedValidation.Equals(environment.NewValidationStatus))
		})
	}
}

type simpleProviderNode struct {
	chainHeadErr   error
	receivedAmount abi.TokenAmount
	receivedErr    error
}

func (s *simpleProviderNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	return shared.TipSetToken{}, 0, s.chainHeadErr
}

func (s *simpleProviderNode) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, tok shared.TipSetToken) (abi.TokenAmount, error) {
	return s.receivedAmount, s.receivedErr
}

func (s *simpleProviderNode) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	panic("not implemented")
}
func (s *simpleProviderNode) GetRetrievalPricingInput(ctx context.Context, pieceCID cid.Cid, storageDeals []abi.DealID) (retrievalmarket.PricingInput, error) {
	panic("not implemented")
}

func TestCancelDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runCancelDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		dealState.ChannelID = &datatransfer.ChannelID{
			Initiator: "initiator",
			Responder: dealState.Receiver,
			ID:        1,
		}
		err := providerstates.CancelDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		dealState.Message = "Existing error"
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "Existing error")
	})
	t.Run("error deleting store", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.DeleteStoreError = errors.New("something went wrong deleting store")
		}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong deleting store")
	})
	t.Run("error closing channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.CloseDataTransferError = errors.New("something went wrong closing")
		}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong closing")
	})
}

func TestCleanupDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runCleanupDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		err := providerstates.CleanupDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusCompleting)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runCleanupDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusCompleted)
	})
	t.Run("error deleting store", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusCompleting)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.DeleteStoreError = errors.New("something went wrong deleting store")
		}
		runCleanupDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong deleting store")
	})

}

var dealID = rm.DealID(10)
var defaultCurrentInterval = uint64(1000)
var defaultIntervalIncrease = uint64(500)
var defaultPricePerByte = abi.NewTokenAmount(500)
var defaultPaymentPerInterval = big.Mul(defaultPricePerByte, abi.NewTokenAmount(int64(defaultCurrentInterval)))
var defaultTotalSent = uint64(5000)
var defaultFundsReceived = abi.NewTokenAmount(2500000)
var defaultUnsealPrice = defaultPaymentPerInterval

func makeDealState(status rm.DealStatus) *rm.ProviderDealState {
	return &rm.ProviderDealState{
		Status:        status,
		FundsReceived: defaultFundsReceived,
		DealProposal: rm.DealProposal{
			ID:     dealID,
			Params: rm.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
		},
	}
}
