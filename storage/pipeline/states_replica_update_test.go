package sealing

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-statemachine"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// handleSubmitReplicaUpdate carries the same FIP-0118 gate as getSectorCollateral: from nv29 the
// pledge must be asked for at full sector size, because every sector gets maximum
// quality-adjusted power regardless of what its pieces claim.
func TestHandleSubmitReplicaUpdateVerifiedSize(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(42)
	const height = abi.ChainEpoch(100)
	const expiration = abi.ChainEpoch(1000)

	sealProof := abi.RegisteredSealProof_StackedDrg8MiBV1_1
	ssize, err := sealProof.SectorSize()
	require.NoError(t, err)

	// a single DDO piece, an eighth of the sector, with no allocation on chain
	pieceSize := abi.PaddedPieceSize(1 << 20)
	sectorPiece := SafePiece(lapi.SectorPiece{
		Piece: abi.PieceInfo{Size: pieceSize, PieceCID: zerocomm.ZeroPieceCommitment(pieceSize.Unpadded())},
		DealInfo: &piece.PieceDealInfo{
			DealID:                  1,
			PieceActivationManifest: &miner.PieceActivationManifest{Size: pieceSize},
		},
	})

	updateSealed, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	sector := SectorInfo{
		SectorNumber:       sectorNumber,
		SectorType:         sealProof,
		CCUpdate:           true,
		Pieces:             []SafeSectorPiece{sectorPiece},
		UpdateSealed:       &updateSealed,
		ReplicaUpdateProof: storiface.ReplicaUpdateProof("proof"),
	}

	unsealed, err := computeUnsealedCIDFromPieces(sector)
	require.NoError(t, err)
	sector.UpdateUnsealed = &unsealed

	for _, tc := range []struct {
		name               string
		nv                 network.Version
		expectVerifiedSize uint64
	}{
		{"nv28 sizes from the pieces", network.Version28, uint64(pieceSize)},
		{"nv29 sizes from the sector", network.Version29, uint64(ssize)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockSealingAPI(ctrl)

			ts := makeTestTipSet(t, height)

			api.EXPECT().ChainHead(gomock.Any()).AnyTimes().Return(ts, nil)
			api.EXPECT().StateSectorPartition(gomock.Any(), maddr, sectorNumber, ts.Key()).
				Return(&miner.SectorLocation{Deadline: 5, Partition: 0}, nil)
			api.EXPECT().StateMinerProvingDeadline(gomock.Any(), maddr, ts.Key()).
				Return(&dline.Info{Index: 0, WPoStPeriodDeadlines: 48}, nil)
			api.EXPECT().StateSectorGetInfo(gomock.Any(), maddr, sectorNumber, ts.Key()).
				Return(&miner.SectorOnChainInfo{Expiration: expiration, InitialPledge: big.Zero()}, nil)
			api.EXPECT().StateNetworkVersion(gomock.Any(), ts.Key()).Return(tc.nv, nil).AnyTimes()

			// the pledge lookup is where the gate shows up, and failing it stops the handler
			// before it needs a working statemachine.Context to send events through
			pledgeErr := errors.New("no pledge for you")
			api.EXPECT().StateMinerInitialPledgeForSector(
				gomock.Any(),
				gomock.Eq(expiration-height),
				gomock.Eq(ssize),
				gomock.Eq(tc.expectVerifiedSize),
				gomock.Eq(ts.Key()),
			).Return(big.Zero(), pledgeErr)

			m := &Sealing{
				Api:       api,
				maddr:     maddr,
				getConfig: func() (sealiface.Config, error) { return sealiface.Config{}, nil },
			}

			err := m.handleSubmitReplicaUpdate(statemachine.Context{}, sector)
			require.ErrorIs(t, err, pledgeErr)
		})
	}
}
