package storageadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	test "github.com/filecoin-project/lotus/chain/events/state/mock"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

func TestOnDealSectorPreCommitted(t *testing.T) {
	provider := address.TestAddress
	ctx := context.Background()
	publishCid := generateCids(1)[0]
	sealedCid := generateCids(1)[0]
	pieceCid := generateCids(1)[0]
	startDealID := abi.DealID(rand.Uint64())
	newDealID := abi.DealID(rand.Uint64())
	newValueReturn := makePublishDealsReturnBytes(t, []abi.DealID{newDealID})
	sectorNumber := abi.SectorNumber(rand.Uint64())
	proposal := market.DealProposal{
		PieceCID:  pieceCid,
		PieceSize: abi.PaddedPieceSize(rand.Uint64()),
		Label:     "success",
	}
	unfinishedDeal := &api.MarketDeal{
		Proposal: proposal,
		State: market.DealState{
			SectorStartEpoch: -1,
			LastUpdatedEpoch: 2,
		},
	}
	successDeal := &api.MarketDeal{
		Proposal: proposal,
		State: market.DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	type testCase struct {
		searchMessageLookup    *api.MsgLookup
		searchMessageErr       error
		checkTsDeals           map[abi.DealID]*api.MarketDeal
		matchStates            []matchState
		dealStartEpochTimeout  bool
		expectedCBCallCount    uint64
		expectedCBSectorNumber abi.SectorNumber
		expectedCBIsActive     bool
		expectedCBError        error
		expectedError          error
	}
	testCases := map[string]testCase{
		"normal sequence": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.PreCommitSector, &miner.SectorPreCommitInfo{
						SectorNumber: sectorNumber,
						SealedCID:    sealedCid,
						DealIDs:      []abi.DealID{startDealID},
					}),
					deals: map[abi.DealID]*api.MarketDeal{
						startDealID: unfinishedDeal,
					},
				},
			},
			expectedCBCallCount:    1,
			expectedCBIsActive:     false,
			expectedCBSectorNumber: sectorNumber,
		},
		"deal id changes in called": {
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				newDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.PreCommitSector, &miner.SectorPreCommitInfo{
						SectorNumber: sectorNumber,
						SealedCID:    sealedCid,
						DealIDs:      []abi.DealID{newDealID},
					}),
					deals: map[abi.DealID]*api.MarketDeal{
						newDealID: unfinishedDeal,
					},
				},
			},
			expectedCBCallCount:    1,
			expectedCBIsActive:     false,
			expectedCBSectorNumber: sectorNumber,
		},
		"error on deal in check": {
			checkTsDeals:        map[abi.DealID]*api.MarketDeal{},
			searchMessageErr:    errors.New("something went wrong"),
			expectedCBCallCount: 0,
			expectedError:       errors.New("failed to set up called handler: failed to look up deal on chain: something went wrong"),
		},
		"sector start epoch > 0 in check": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: successDeal,
			},
			expectedCBCallCount: 1,
			expectedCBIsActive:  true,
		},
		"error on deal in pre-commit": {
			searchMessageErr: errors.New("something went wrong"),
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.PreCommitSector, &miner.SectorPreCommitInfo{
						SectorNumber: sectorNumber,
						SealedCID:    sealedCid,
						DealIDs:      []abi.DealID{startDealID},
					}),
					deals: map[abi.DealID]*api.MarketDeal{},
				},
			},
			expectedCBCallCount: 1,
			expectedCBError:     errors.New("handling applied event: something went wrong"),
			expectedError:       errors.New("failed to set up called handler: something went wrong"),
		},
		"proposed deal epoch timeout": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			dealStartEpochTimeout: true,
			expectedCBCallCount:   1,
			expectedCBError:       xerrors.Errorf("handling applied event: deal %d was not activated by proposed deal start epoch 0", startDealID),
		},
	}
	runTestCase := func(testCase string, data testCase) {
		t.Run(testCase, func(t *testing.T) {
			//	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			//	defer cancel()
			api := &mockGetCurrentDealInfoAPI{
				SearchMessageLookup: data.searchMessageLookup,
				SearchMessageErr:    data.searchMessageErr,
				MarketDeals:         make(map[marketDealKey]*api.MarketDeal),
			}
			checkTs, err := test.MockTipset(provider, rand.Uint64())
			require.NoError(t, err)
			for dealID, deal := range data.checkTsDeals {
				api.MarketDeals[marketDealKey{dealID, checkTs.Key()}] = deal
			}
			matchMessages := make([]matchMessage, len(data.matchStates))
			for i, ms := range data.matchStates {
				matchTs, err := test.MockTipset(provider, rand.Uint64())
				require.NoError(t, err)
				for dealID, deal := range ms.deals {
					api.MarketDeals[marketDealKey{dealID, matchTs.Key()}] = deal
				}
				matchMessages[i] = matchMessage{
					curH:       5,
					msg:        ms.msg,
					msgReceipt: nil,
					ts:         matchTs,
				}
			}
			eventsAPI := &fakeEvents{
				Ctx:                   ctx,
				CheckTs:               checkTs,
				MatchMessages:         matchMessages,
				DealStartEpochTimeout: data.dealStartEpochTimeout,
			}
			cbCallCount := uint64(0)
			var cbSectorNumber abi.SectorNumber
			var cbIsActive bool
			var cbError error
			cb := func(secNum abi.SectorNumber, isActive bool, err error) {
				cbCallCount++
				cbSectorNumber = secNum
				cbIsActive = isActive
				cbError = err
			}
			err = OnDealSectorPreCommitted(ctx, api, eventsAPI, provider, startDealID, proposal, &publishCid, cb)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, data.expectedError.Error())
			}
			require.Equal(t, data.expectedCBSectorNumber, cbSectorNumber)
			require.Equal(t, data.expectedCBIsActive, cbIsActive)
			require.Equal(t, data.expectedCBCallCount, cbCallCount)
			if data.expectedCBError == nil {
				require.NoError(t, cbError)
			} else {
				require.EqualError(t, cbError, data.expectedCBError.Error())
			}
		})
	}
	for testCase, data := range testCases {
		runTestCase(testCase, data)
	}
}

func TestOnDealSectorCommitted(t *testing.T) {
	provider := address.TestAddress
	ctx := context.Background()
	publishCid := generateCids(1)[0]
	pieceCid := generateCids(1)[0]
	startDealID := abi.DealID(rand.Uint64())
	newDealID := abi.DealID(rand.Uint64())
	newValueReturn := makePublishDealsReturnBytes(t, []abi.DealID{newDealID})
	sectorNumber := abi.SectorNumber(rand.Uint64())
	proposal := market.DealProposal{
		PieceCID:  pieceCid,
		PieceSize: abi.PaddedPieceSize(rand.Uint64()),
		Label:     "success",
	}
	unfinishedDeal := &api.MarketDeal{
		Proposal: proposal,
		State: market.DealState{
			SectorStartEpoch: -1,
			LastUpdatedEpoch: 2,
		},
	}
	successDeal := &api.MarketDeal{
		Proposal: proposal,
		State: market.DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	type testCase struct {
		searchMessageLookup   *api.MsgLookup
		searchMessageErr      error
		checkTsDeals          map[abi.DealID]*api.MarketDeal
		matchStates           []matchState
		dealStartEpochTimeout bool
		expectedCBCallCount   uint64
		expectedCBError       error
		expectedError         error
	}
	testCases := map[string]testCase{
		"normal sequence": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.ProveCommitSector, &miner.ProveCommitSectorParams{
						SectorNumber: sectorNumber,
					}),
					deals: map[abi.DealID]*api.MarketDeal{
						startDealID: successDeal,
					},
				},
			},
			expectedCBCallCount: 1,
		},
		"deal id changes in called": {
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				newDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.ProveCommitSector, &miner.ProveCommitSectorParams{
						SectorNumber: sectorNumber,
					}),
					deals: map[abi.DealID]*api.MarketDeal{
						newDealID: successDeal,
					},
				},
			},
			expectedCBCallCount: 1,
		},
		"error on deal in check": {
			checkTsDeals:        map[abi.DealID]*api.MarketDeal{},
			searchMessageErr:    errors.New("something went wrong"),
			expectedCBCallCount: 0,
			expectedError:       errors.New("failed to set up called handler: failed to look up deal on chain: something went wrong"),
		},
		"sector start epoch > 0 in check": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: successDeal,
			},
			expectedCBCallCount: 1,
		},
		"error on deal in called": {
			searchMessageErr: errors.New("something went wrong"),
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			matchStates: []matchState{
				{
					msg: makeMessage(t, provider, miner.Methods.ProveCommitSector, &miner.ProveCommitSectorParams{
						SectorNumber: sectorNumber,
					}),
					deals: map[abi.DealID]*api.MarketDeal{
						newDealID: successDeal,
					},
				},
			},
			expectedCBCallCount: 1,
			expectedCBError:     errors.New("handling applied event: failed to look up deal on chain: something went wrong"),
			expectedError:       errors.New("failed to set up called handler: failed to look up deal on chain: something went wrong"),
		},
		"proposed deal epoch timeout": {
			checkTsDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: unfinishedDeal,
			},
			dealStartEpochTimeout: true,
			expectedCBCallCount:   1,
			expectedCBError:       xerrors.Errorf("handling applied event: deal %d was not activated by proposed deal start epoch 0", startDealID),
		},
	}
	runTestCase := func(testCase string, data testCase) {
		t.Run(testCase, func(t *testing.T) {
			//	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			//	defer cancel()
			api := &mockGetCurrentDealInfoAPI{
				SearchMessageLookup: data.searchMessageLookup,
				SearchMessageErr:    data.searchMessageErr,
				MarketDeals:         make(map[marketDealKey]*api.MarketDeal),
			}
			checkTs, err := test.MockTipset(provider, rand.Uint64())
			require.NoError(t, err)
			for dealID, deal := range data.checkTsDeals {
				api.MarketDeals[marketDealKey{dealID, checkTs.Key()}] = deal
			}
			matchMessages := make([]matchMessage, len(data.matchStates))
			for i, ms := range data.matchStates {
				matchTs, err := test.MockTipset(provider, rand.Uint64())
				require.NoError(t, err)
				for dealID, deal := range ms.deals {
					api.MarketDeals[marketDealKey{dealID, matchTs.Key()}] = deal
				}
				matchMessages[i] = matchMessage{
					curH:       5,
					msg:        ms.msg,
					msgReceipt: nil,
					ts:         matchTs,
				}
			}
			eventsAPI := &fakeEvents{
				Ctx:                   ctx,
				CheckTs:               checkTs,
				MatchMessages:         matchMessages,
				DealStartEpochTimeout: data.dealStartEpochTimeout,
			}
			cbCallCount := uint64(0)
			var cbError error
			cb := func(err error) {
				cbCallCount++
				cbError = err
			}
			err = OnDealSectorCommitted(ctx, api, eventsAPI, provider, startDealID, sectorNumber, proposal, &publishCid, cb)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, data.expectedError.Error())
			}
			require.Equal(t, data.expectedCBCallCount, cbCallCount)
			if data.expectedCBError == nil {
				require.NoError(t, cbError)
			} else {
				require.EqualError(t, cbError, data.expectedCBError.Error())
			}
		})
	}
	for testCase, data := range testCases {
		runTestCase(testCase, data)
	}
}

type matchState struct {
	msg   *types.Message
	deals map[abi.DealID]*api.MarketDeal
}

type matchMessage struct {
	curH       abi.ChainEpoch
	msg        *types.Message
	msgReceipt *types.MessageReceipt
	ts         *types.TipSet
	doesRevert bool
}
type fakeEvents struct {
	Ctx                   context.Context
	CheckTs               *types.TipSet
	MatchMessages         []matchMessage
	DealStartEpochTimeout bool
}

func (fe *fakeEvents) Called(check events.CheckFunc, msgHnd events.MsgHandler, rev events.RevertHandler, confidence int, timeout abi.ChainEpoch, mf events.MsgMatchFunc) error {
	if fe.DealStartEpochTimeout {
		msgHnd(nil, nil, nil, 100) // nolint:errcheck
		return nil
	}

	_, more, err := check(fe.CheckTs)
	if err != nil {
		return err
	}
	if !more {
		return nil
	}
	for _, matchMessage := range fe.MatchMessages {
		matched, err := mf(matchMessage.msg)
		if err != nil {
			return err
		}
		if matched {
			more, err := msgHnd(matchMessage.msg, matchMessage.msgReceipt, matchMessage.ts, matchMessage.curH)
			if err != nil {
				return err
			}
			if matchMessage.doesRevert {
				err := rev(fe.Ctx, matchMessage.ts)
				if err != nil {
					return err
				}
			}
			if !more {
				return nil
			}
		}
	}
	return nil
}

func makeMessage(t *testing.T, to address.Address, method abi.MethodNum, params cbor.Marshaler) *types.Message {
	buf := new(bytes.Buffer)
	err := params.MarshalCBOR(buf)
	require.NoError(t, err)
	return &types.Message{
		To:     to,
		Method: method,
		Params: buf.Bytes(),
	}
}

var seq int

func generateCids(n int) []cid.Cid {
	cids := make([]cid.Cid, 0, n)
	for i := 0; i < n; i++ {
		c := blocks.NewBlock([]byte(fmt.Sprint(seq))).Cid()
		seq++
		cids = append(cids, c)
	}
	return cids
}
