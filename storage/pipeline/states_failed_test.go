package sealing_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-statemachine"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	api2 "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	pipeline "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

func TestStateRecoverDealIDs(t *testing.T) {
	t.Skip("Bring this back when we can correctly mock a state machine context: Issue #7867")
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	api := mocks.NewMockSealingAPI(mockCtrl)

	fakeSealing := &pipeline.Sealing{
		Api:      api,
		DealInfo: &pipeline.CurrentDealInfoManager{CDAPI: api},
	}

	sctx := mocks.NewMockContext(mockCtrl)
	sctx.EXPECT().Context().AnyTimes().Return(ctx)

	api.EXPECT().ChainHead(ctx).Times(2).Return(nil, abi.ChainEpoch(10), nil)

	var dealId abi.DealID = 12
	dealProposal := market.DealProposal{
		PieceCID: idCid("newPieceCID"),
	}

	api.EXPECT().StateMarketStorageDeal(ctx, dealId, nil).Return(&api2.MarketDeal{Proposal: dealProposal}, nil)

	pc := idCid("publishCID")

	// expect GetCurrentDealInfo
	{
		api.EXPECT().StateSearchMsg(ctx, gomock.Any(), pc, gomock.Any(), gomock.Any()).Return(&api2.MsgLookup{
			Receipt: types.MessageReceipt{
				ExitCode: exitcode.Ok,
				Return: cborRet(&market0.PublishStorageDealsReturn{
					IDs: []abi.DealID{dealId},
				}),
			},
		}, nil)
		api.EXPECT().StateNetworkVersion(ctx, nil).Return(network.Version0, nil)
		api.EXPECT().StateMarketStorageDeal(ctx, dealId, nil).Return(&api2.MarketDeal{
			Proposal: dealProposal,
		}, nil)

	}

	sctx.EXPECT().Send(pipeline.SectorRemove{}).Return(nil)

	// TODO sctx should satisfy an interface so it can be usable for mocking.  This will fail because we are passing in an empty context now to get this to build.
	// https://github.com/filecoin-project/lotus/issues/7867
	err := fakeSealing.HandleRecoverDealIDs(statemachine.Context{}, pipeline.SectorInfo{
		Pieces: []pipeline.SafeSectorPiece{
			pipeline.SafePiece(api2.SectorPiece{
				DealInfo: &piece.PieceDealInfo{
					DealID:     dealId,
					PublishCid: &pc,
				},
				Piece: abi.PieceInfo{
					PieceCID: idCid("oldPieceCID"),
				},
			}),
		},
	})
	require.NoError(t, err)
}

func idCid(str string) cid.Cid {
	builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
	c, err := builder.Sum([]byte(str))
	if err != nil {
		panic(err)
	}
	return c
}

func cborRet(v cbor.Marshaler) []byte {
	var buf bytes.Buffer
	if err := v.MarshalCBOR(&buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
