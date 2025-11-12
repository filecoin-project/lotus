package sealing

import (
	"bytes"
	"errors"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	evtmock "github.com/filecoin-project/lotus/chain/events/state/mock"
	"github.com/filecoin-project/lotus/chain/types"
)

var errNotFound = errors.New("could not find")

func TestGetCurrentDealInfo(t *testing.T) {
	success, err := markettypes.NewLabelFromString("success")
	require.NoError(t, err)

	other, err := markettypes.NewLabelFromString("other")
	require.NoError(t, err)

	another, err := markettypes.NewLabelFromString("another")
	require.NoError(t, err)

	ctx := context.Background()
	dummyCid, _ := cid.Parse("bafkqaaa")
	dummyCid2, _ := cid.Parse("bafkqaab")
	zeroDealID := abi.DealID(0)
	anotherDealID := abi.DealID(8)
	earlierDealID := abi.DealID(9)
	successDealID := abi.DealID(10)
	proposal := market.DealProposal{
		PieceCID:             dummyCid,
		PieceSize:            abi.PaddedPieceSize(100),
		Client:               tutils.NewActorAddr(t, "client"),
		Provider:             tutils.NewActorAddr(t, "provider"),
		StoragePricePerEpoch: abi.NewTokenAmount(1),
		ProviderCollateral:   abi.NewTokenAmount(1),
		ClientCollateral:     abi.NewTokenAmount(1),
		Label:                success,
	}
	otherProposal := market.DealProposal{
		PieceCID:             dummyCid2,
		PieceSize:            abi.PaddedPieceSize(100),
		Client:               tutils.NewActorAddr(t, "client"),
		Provider:             tutils.NewActorAddr(t, "provider"),
		StoragePricePerEpoch: abi.NewTokenAmount(1),
		ProviderCollateral:   abi.NewTokenAmount(1),
		ClientCollateral:     abi.NewTokenAmount(1),
		Label:                other,
	}
	anotherProposal := market.DealProposal{
		PieceCID:             dummyCid2,
		PieceSize:            abi.PaddedPieceSize(100),
		Client:               tutils.NewActorAddr(t, "client"),
		Provider:             tutils.NewActorAddr(t, "provider"),
		StoragePricePerEpoch: abi.NewTokenAmount(1),
		ProviderCollateral:   abi.NewTokenAmount(1),
		ClientCollateral:     abi.NewTokenAmount(1),
		Label:                another,
	}
	successDeal := &api.MarketDeal{
		Proposal: proposal,
		State: api.MarketDealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	earlierDeal := &api.MarketDeal{
		Proposal: otherProposal,
		State: api.MarketDealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}
	anotherDeal := &api.MarketDeal{
		Proposal: anotherProposal,
		State: api.MarketDealState{
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
		},
	}

	type testCaseData struct {
		searchMessageLookup *api.MsgLookup
		searchMessageErr    error
		marketDeals         map[abi.DealID]*api.MarketDeal
		publishCid          cid.Cid
		targetProposal      *market.DealProposal
		expectedDealID      abi.DealID
		expectedMarketDeal  *api.MarketDeal
		expectedError       error
		networkVersion      network.Version
	}
	testCases := map[string]testCaseData{
		"deal lookup succeeds": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{successDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				successDealID: successDeal,
			},
			targetProposal:     &proposal,
			expectedDealID:     successDealID,
			expectedMarketDeal: successDeal,
		},
		"deal lookup succeeds two return values": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{earlierDealID, successDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				earlierDealID: earlierDeal,
				successDealID: successDeal,
			},
			targetProposal:     &proposal,
			expectedDealID:     successDealID,
			expectedMarketDeal: successDeal,
		},
		"deal lookup fails proposal mismatch": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{earlierDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				earlierDealID: earlierDeal,
			},
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("could not find deal in publish deals message %s", dummyCid),
		},
		"deal lookup handles invalid actor output with mismatched count of deals and return values": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{earlierDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				earlierDealID: earlierDeal,
				successDealID: successDeal,
			},
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("invalid publish storage deals ret marking 1 as valid while only returning 1 valid deals in publish deal message %s", dummyCid),
		},

		"deal lookup fails when deal was not valid and index exceeds output array": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturn(t, []abi.DealID{earlierDealID}, []uint64{0}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				earlierDealID: earlierDeal,
				successDealID: successDeal,
			},
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("deal was invalid at publication"),
			networkVersion: network.Version14,
		},

		"deal lookup succeeds when theres a separate deal failure": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturn(t, []abi.DealID{anotherDealID, successDealID}, []uint64{0, 2}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				anotherDealID: anotherDeal,
				earlierDealID: earlierDeal,
				successDealID: successDeal,
			},
			targetProposal:     &proposal,
			expectedDealID:     successDealID,
			expectedMarketDeal: successDeal,
			networkVersion:     network.Version14,
		},

		"deal lookup succeeds, target proposal nil, single deal in message": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{successDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				successDealID: successDeal,
			},
			targetProposal:     nil,
			expectedDealID:     successDealID,
			expectedMarketDeal: successDeal,
		},
		"deal lookup fails, multiple deals in return value but target proposal nil": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   makePublishDealsReturnBytesOldVersion(t, []abi.DealID{earlierDealID, successDealID}),
				},
			},
			marketDeals: map[abi.DealID]*api.MarketDeal{
				earlierDealID: earlierDeal,
				successDealID: successDeal,
			},
			targetProposal: nil,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("getting deal ID from publish deal message %s: no deal proposal supplied but message return value has more than one deal (2 deals)", dummyCid),
		},
		"search message fails": {
			publishCid:       dummyCid,
			searchMessageErr: errors.New("something went wrong"),
			targetProposal:   &proposal,
			expectedDealID:   zeroDealID,
			expectedError:    xerrors.Errorf("looking for publish deal message %s: search msg failed: something went wrong", dummyCid),
		},
		"search message not found": {
			publishCid:     dummyCid,
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("looking for publish deal message %s: not found", dummyCid),
		},
		"return code not ok": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.ErrIllegalState,
				},
			},
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("looking for publish deal message %s: non-ok exit code: %s", dummyCid, exitcode.ErrIllegalState),
		},
		"unable to unmarshal params": {
			publishCid: dummyCid,
			searchMessageLookup: &api.MsgLookup{
				Receipt: types.MessageReceipt{
					ExitCode: exitcode.Ok,
					Return:   []byte("applesauce"),
				},
			},
			targetProposal: &proposal,
			expectedDealID: zeroDealID,
			expectedError:  xerrors.Errorf("looking for publish deal message %s: decoding message return: failed to unmarshal PublishStorageDealsReturn: cbor input should be of type array", dummyCid),
		},
	}
	runTestCase := func(testCase string, data testCaseData) {
		t.Run(testCase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ts, err := evtmock.MockTipset(address.TestAddress, rand.Uint64())
			require.NoError(t, err)
			marketDeals := make(map[marketDealKey]*api.MarketDeal)
			for dealID, deal := range data.marketDeals {
				marketDeals[marketDealKey{dealID, ts.Key()}] = deal
			}
			mockApi := &CurrentDealInfoMockAPI{
				SearchMessageLookup: data.searchMessageLookup,
				SearchMessageErr:    data.searchMessageErr,
				MarketDeals:         marketDeals,
				Version:             data.networkVersion,
			}
			dealInfoMgr := CurrentDealInfoManager{mockApi}

			res, err := dealInfoMgr.GetCurrentDealInfo(ctx, ts.Key(), data.targetProposal, data.publishCid)
			require.Equal(t, data.expectedDealID, res.DealID)
			require.Equal(t, data.expectedMarketDeal, res.MarketDeal)
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

type CurrentDealInfoMockAPI struct {
	SearchMessageLookup *api.MsgLookup
	SearchMessageErr    error

	MarketDeals map[marketDealKey]*api.MarketDeal
	Version     network.Version
}

func (mapi *CurrentDealInfoMockAPI) ChainGetMessage(ctx context.Context, c cid.Cid) (*types.Message, error) {
	var keys []marketDealKey
	for k := range mapi.MarketDeals {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i].DealID < keys[j].DealID
	})

	var deals []markettypes.ClientDealProposal
	for _, k := range keys {
		dl := mapi.MarketDeals[k]
		deals = append(deals, markettypes.ClientDealProposal{
			Proposal: dl.Proposal,
			ClientSignature: crypto.Signature{
				Data: []byte("foo bar cat dog"),
				Type: crypto.SigTypeBLS,
			},
		})
	}

	buf := new(bytes.Buffer)
	params := markettypes.PublishStorageDealsParams{Deals: deals}
	err := params.MarshalCBOR(buf)
	if err != nil {
		panic(err)
	}

	return &types.Message{
		Params: buf.Bytes(),
	}, nil
}

func (mapi *CurrentDealInfoMockAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return addr, nil
}

func (mapi *CurrentDealInfoMockAPI) StateMarketStorageDeal(ctx context.Context, dealID abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	deal, ok := mapi.MarketDeals[marketDealKey{dealID, tsk}]
	if !ok {
		return nil, errNotFound
	}
	return deal, nil
}

func (mapi *CurrentDealInfoMockAPI) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if mapi.SearchMessageLookup == nil {
		return mapi.SearchMessageLookup, mapi.SearchMessageErr
	}

	return mapi.SearchMessageLookup, mapi.SearchMessageErr
}

func (mapi *CurrentDealInfoMockAPI) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	return mapi.Version, nil
}

func makePublishDealsReturnBytesOldVersion(t *testing.T, dealIDs []abi.DealID) []byte {
	buf := new(bytes.Buffer)
	dealsReturn := market0.PublishStorageDealsReturn{
		IDs: dealIDs,
	}
	err := dealsReturn.MarshalCBOR(buf)
	require.NoError(t, err)

	return buf.Bytes()
}

func makePublishDealsReturn(t *testing.T, dealIDs []abi.DealID, validIdxs []uint64) []byte {
	buf := new(bytes.Buffer)
	dealsReturn := markettypes.PublishStorageDealsReturn{
		IDs:        dealIDs,
		ValidDeals: bitfield.NewFromSet(validIdxs),
	}
	err := dealsReturn.MarshalCBOR(buf)
	require.NoError(t, err)

	return buf.Bytes()
}
