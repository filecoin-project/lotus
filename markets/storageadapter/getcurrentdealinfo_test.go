package storageadapter

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	test "github.com/filecoin-project/lotus/chain/events/state/mock"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

var errNotFound = errors.New("Could not find")

func TestGetCurrentDealInfo(t *testing.T) {
	ctx := context.Background()
	dummyCid, _ := cid.Parse("bafkqaaa")
	startDealID := abi.DealID(rand.Uint64())
	newDealID := abi.DealID(rand.Uint64())
	twoValuesReturn := makePublishDealsReturnBytes(t, []abi.DealID{abi.DealID(rand.Uint64()), abi.DealID(rand.Uint64())})
	sameValueReturn := makePublishDealsReturnBytes(t, []abi.DealID{startDealID})
	newValueReturn := makePublishDealsReturnBytes(t, []abi.DealID{newDealID})
	proposal := market.DealProposal{
		PieceCID:  dummyCid,
		PieceSize: abi.PaddedPieceSize(rand.Uint64()),
		Label:     "success",
	}
	otherProposal := market.DealProposal{
		PieceCID:  dummyCid,
		PieceSize: abi.PaddedPieceSize(rand.Uint64()),
		Label:     "other",
	}
	successDeal := &api.MarketDeal{
		Proposal: proposal,
		State: market.DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	otherDeal := &api.MarketDeal{
		Proposal: otherProposal,
		State: market.DealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	testCases := map[string]struct {
		searchMessageLookup *api.MsgLookup
		searchMessageErr    error
		marketDeals         map[abi.DealID]*api.MarketDeal
		publishCid          *cid.Cid
		expectedDealID      abi.DealID
		expectedMarketDeal  *api.MarketDeal
		expectedError       error
	}{
		"deal lookup succeeds": {
			marketDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: successDeal,
			},
			expectedDealID:     startDealID,
			expectedMarketDeal: successDeal,
		},
		"publish CID = nil": {
			expectedDealID: startDealID,
			expectedError:  errNotFound,
		},
		"publish CID = nil, other deal on lookup": {
			marketDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: otherDeal,
			},
			expectedDealID: startDealID,
			expectedError:  xerrors.Errorf("Deal proposals did not match"),
		},
		"search message fails": {
			publishCid:       &dummyCid,
			searchMessageErr: errors.New("something went wrong"),
			expectedDealID:   startDealID,
			expectedError:    errors.New("something went wrong"),
		},
		"return code not ok": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.ErrIllegalState,
				},
			},
			expectedDealID: startDealID,
			expectedError:  xerrors.Errorf("looking for publish deal message %s: non-ok exit code: %s", dummyCid, exitcode.ErrIllegalState),
		},
		"unable to unmarshal params": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   []byte("applesauce"),
				},
			},
			expectedDealID: startDealID,
			expectedError:  xerrors.Errorf("looking for publish deal message: unmarshaling message return: cbor input should be of type array"),
		},
		"more than one returned id": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   twoValuesReturn,
				},
			},
			expectedDealID: startDealID,
			expectedError:  xerrors.Errorf("can't recover dealIDs from publish deal message with more than 1 deal"),
		},
		"deal ids still match": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   sameValueReturn,
				},
			},
			expectedDealID: startDealID,
			expectedError:  errNotFound,
		},
		"new deal id success": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				newDealID: successDeal,
			},
			expectedDealID:     newDealID,
			expectedMarketDeal: successDeal,
		},
		"new deal id after other deal found": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				startDealID: otherDeal,
				newDealID:   successDeal,
			},
			expectedDealID:     newDealID,
			expectedMarketDeal: successDeal,
		},
		"new deal id failure": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			expectedDealID: newDealID,
			expectedError:  errNotFound,
		},
		"new deal id, failure due to other deal present": {
			publishCid: &dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   newValueReturn,
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				newDealID: otherDeal,
			},
			expectedDealID: newDealID,
			expectedError:  xerrors.Errorf("Deal proposals did not match"),
		},
	}
	runTestCase := func(testCase string, data struct {
		searchMessageLookup *api.MsgLookup
		searchMessageErr    error
		marketDeals         map[abi.DealID]*api.MarketDeal
		publishCid          *cid.Cid
		expectedDealID      abi.DealID
		expectedMarketDeal  *api.MarketDeal
		expectedError       error
	}) {
		t.Run(testCase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ts, err := test.MockTipset(address.TestAddress, rand.Uint64())
			require.NoError(t, err)
			marketDeals := make(map[marketDealKey]*api.MarketDeal)
			for dealID, deal := range data.marketDeals {
				marketDeals[marketDealKey{dealID, ts.Key()}] = deal
			}
			api := &mockGetCurrentDealInfoAPI{
				SearchMessageLookup: data.searchMessageLookup,
				SearchMessageErr:    data.searchMessageErr,
				MarketDeals:         marketDeals,
			}

			dealID, marketDeal, err := GetCurrentDealInfo(ctx, ts, api, startDealID, proposal, data.publishCid)
			require.Equal(t, data.expectedDealID, dealID)
			require.Equal(t, data.expectedMarketDeal, marketDeal)
			if data.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, data.expectedError.Error())
			}
		})
	}
	for testCase, data := range testCases {
		runTestCase(testCase, data)
	}
}

type marketDealKey struct {
	abi.DealID
	types.TipSetKey
}

type mockGetCurrentDealInfoAPI struct {
	SearchMessageLookup *api.MsgLookup
	SearchMessageErr    error

	MarketDeals map[marketDealKey]*api.MarketDeal
}

func (mapi *mockGetCurrentDealInfoAPI) StateMarketStorageDeal(ctx context.Context, dealID abi.DealID, ts types.TipSetKey) (*api.MarketDeal, error) {
	deal, ok := mapi.MarketDeals[marketDealKey{dealID, ts}]
	if !ok {
		return nil, errNotFound
	}
	return deal, nil
}

func (mapi *mockGetCurrentDealInfoAPI) StateSearchMsg(context.Context, cid.Cid) (*api.MsgLookup, error) {
	return mapi.SearchMessageLookup, mapi.SearchMessageErr
}

func (mapi *mockGetCurrentDealInfoAPI) StateLookupID(ctx context.Context, addr address.Address, ts types.TipSetKey) (address.Address, error) {
	return addr, nil
}

func makePublishDealsReturnBytes(t *testing.T, dealIDs []abi.DealID) []byte {
	buf := new(bytes.Buffer)
	dealsReturn := market.PublishStorageDealsReturn{
		IDs: dealIDs,
	}
	err := dealsReturn.MarshalCBOR(buf)
	require.NoError(t, err)
	return buf.Bytes()
}
