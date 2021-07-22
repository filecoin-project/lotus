package retrievaladapter

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	testnet "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestGetPricingInput(t *testing.T) {
	ctx := context.Background()
	tsk := &types.TipSet{}
	key := tsk.Key()

	pcid := testnet.GenerateCids(1)[0]
	deals := []abi.DealID{1, 2}
	paddedSize := abi.PaddedPieceSize(128)
	unpaddedSize := paddedSize.Unpadded()

	tcs := map[string]struct {
		pieceCid cid.Cid
		deals    []abi.DealID
		fFnc     func(node *mocks.MockFullNode)

		expectedErrorStr  string
		expectedVerified  bool
		expectedPieceSize abi.UnpaddedPieceSize
	}{
		"error when fails to fetch chain head": {
			fFnc: func(n *mocks.MockFullNode) {
				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, xerrors.New("chain head error")).Times(1)
			},
			expectedErrorStr: "chain head error",
		},

		"error when no piece matches": {
			fFnc: func(n *mocks.MockFullNode) {
				out1 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID: testnet.GenerateCids(1)[0],
					},
				}
				out2 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID: testnet.GenerateCids(1)[0],
					},
				}

				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, nil).Times(1)
				gomock.InOrder(
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[0], key).Return(out1, nil),
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[1], key).Return(out2, nil),
				)

			},
			expectedErrorStr: "failed to find matching piece",
		},

		"error when fails to fetch deal state": {
			fFnc: func(n *mocks.MockFullNode) {
				out1 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:  pcid,
						PieceSize: paddedSize,
					},
				}
				out2 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:     testnet.GenerateCids(1)[0],
						VerifiedDeal: true,
					},
				}

				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, nil).Times(1)
				gomock.InOrder(
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[0], key).Return(out1, xerrors.New("error 1")),
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[1], key).Return(out2, xerrors.New("error 2")),
				)

			},
			expectedErrorStr: "failed to fetch storage deal state",
		},

		"verified is true even if one deal is verified and we get the correct piecesize": {
			fFnc: func(n *mocks.MockFullNode) {
				out1 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:  pcid,
						PieceSize: paddedSize,
					},
				}
				out2 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:     testnet.GenerateCids(1)[0],
						VerifiedDeal: true,
					},
				}

				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, nil).Times(1)
				gomock.InOrder(
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[0], key).Return(out1, nil),
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[1], key).Return(out2, nil),
				)

			},
			expectedPieceSize: unpaddedSize,
			expectedVerified:  true,
		},

		"success even if one deal state fetch errors out but the other deal is verified and has the required piececid": {
			fFnc: func(n *mocks.MockFullNode) {
				out1 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID: testnet.GenerateCids(1)[0],
					},
				}
				out2 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:     pcid,
						PieceSize:    paddedSize,
						VerifiedDeal: true,
					},
				}

				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, nil).Times(1)
				gomock.InOrder(
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[0], key).Return(out1, xerrors.New("some error")),
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[1], key).Return(out2, nil),
				)

			},
			expectedPieceSize: unpaddedSize,
			expectedVerified:  true,
		},

		"verified is false if both deals are unverified and we get the correct piece size": {
			fFnc: func(n *mocks.MockFullNode) {
				out1 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:     pcid,
						PieceSize:    paddedSize,
						VerifiedDeal: false,
					},
				}
				out2 := &api.MarketDeal{
					Proposal: market.DealProposal{
						PieceCID:     testnet.GenerateCids(1)[0],
						VerifiedDeal: false,
					},
				}

				n.EXPECT().ChainHead(gomock.Any()).Return(tsk, nil).Times(1)
				gomock.InOrder(
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[0], key).Return(out1, nil),
					n.EXPECT().StateMarketStorageDeal(gomock.Any(), deals[1], key).Return(out2, nil),
				)

			},
			expectedPieceSize: unpaddedSize,
			expectedVerified:  false,
		},
	}

	for name, tc := range tcs {
		tc := tc
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()

			mockFull := mocks.NewMockFullNode(mockCtrl)
			rpn := &retrievalProviderNode{
				full: mockFull,
			}
			if tc.fFnc != nil {
				tc.fFnc(mockFull)
			}

			resp, err := rpn.GetRetrievalPricingInput(ctx, pcid, deals)

			if tc.expectedErrorStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErrorStr)
				require.Equal(t, retrievalmarket.PricingInput{}, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedPieceSize, resp.PieceSize)
				require.Equal(t, tc.expectedVerified, resp.VerifiedDeal)
			}
		})
	}
}
