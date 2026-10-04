package sealing

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

// fakeSealingCtx records the events maybeStartSealing sends.
type fakeSealingCtx struct {
	sent []interface{}
}

func (f *fakeSealingCtx) Context() context.Context { return context.Background() }

func (f *fakeSealingCtx) Send(evt interface{}) error {
	f.sent = append(f.sent, evt)
	return nil
}

// fakeSealingAPI implements only what maybeStartSealing uses; the embedded
// interface satisfies the rest of SealingAPI and panics if anything else is
// called unexpectedly.
type fakeSealingAPI struct {
	SealingAPI
	t          *testing.T
	headHeight abi.ChainEpoch
	nv         network.Version
}

func (f *fakeSealingAPI) ChainHead(context.Context) (*types.TipSet, error) {
	return makeTestTipSet(f.t, f.headHeight), nil
}

func (f *fakeSealingAPI) StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error) {
	return f.nv, nil
}

func dealPiece(t *testing.T, startEpoch abi.ChainEpoch) SafeSectorPiece {
	c, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	return SafePiece(api.SectorPiece{
		Piece: abi.PieceInfo{
			Size:     abi.PaddedPieceSize(1024),
			PieceCID: c,
		},
		DealInfo: &piece.PieceDealInfo{
			DealID: abi.DealID(1),
			DealSchedule: piece.DealSchedule{
				StartEpoch: startEpoch,
				EndEpoch:   startEpoch + 10000,
			},
		},
	})
}

// TestMaybeStartSealingEarliestDeadline checks that a sector with several deals
// starts sealing before the earliest of their deadlines, not before the one
// belonging to the last piece iterated. Regression test: the deadline used to
// be overwritten for every piece, so only the last piece's deal deadline was
// honoured.
func TestMaybeStartSealingEarliestDeadline(t *testing.T) {
	const (
		buffer     = abi.ChainEpoch(10)
		headHeight = abi.ChainEpoch(85)
		earlyStart = abi.ChainEpoch(50)  // safe seal epoch 40, already in the past
		lateStart  = abi.ChainEpoch(100) // safe seal epoch 90, still in the future
	)

	sapi := &fakeSealingAPI{t: t, headHeight: headHeight, nv: network.Version29}

	maddr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	m := &Sealing{
		Api:          sapi,
		maddr:        maddr,
		sectorTimers: map[abi.SectorID]*time.Timer{},
		getConfig: func() (sealiface.Config, error) {
			return sealiface.Config{
				WaitDealsDelay:          time.Hour,
				StartEpochSealingBuffer: buffer,
			}, nil
		},
	}

	sector := SectorInfo{
		SectorNumber: 1,
		SectorType:   abi.RegisteredSealProof_StackedDrg32GiBV1_1,
		CreationTime: time.Now().Unix(),
		// Earliest deal first, latest last: with last-piece-wins the latest
		// deal would set the deadline and the sector would keep waiting.
		Pieces: []SafeSectorPiece{
			dealPiece(t, earlyStart),
			dealPiece(t, lateStart),
		},
	}

	ctx := &fakeSealingCtx{}
	started, err := m.maybeStartSealing(ctx, sector, 2*1024)
	require.NoError(t, err)
	require.True(t, started, "sector should start sealing before the earliest deal deadline")
	require.Len(t, ctx.sent, 1)
	require.IsType(t, SectorStartPacking{}, ctx.sent[0])
}
