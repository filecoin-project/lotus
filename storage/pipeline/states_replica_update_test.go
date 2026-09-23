package sealing

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

// replicaUpdatePledge funds the pledge ProveReplicaUpdates3 re-derives, with 10% headroom on any
// top-up. Before nv29 the update's power follows the verified allocations it claims. From nv29
// (FIP-0118) it grants maximum quality-adjusted power, re-deriving the pledge only for a sector
// not already there.
func TestReplicaUpdatePledge(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(42)
	const height = abi.ChainEpoch(100)
	const expiration = abi.ChainEpoch(1000)

	sealProof := abi.RegisteredSealProof_StackedDrg8MiBV1_1
	ssize, err := sealProof.SectorSize()
	require.NoError(t, err)

	pieceCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)
	pieceSize := abi.PaddedPieceSize(1 << 20)

	allocKey := &miner.VerifiedAllocationKey{Client: 1000, ID: 7}
	allocClient, err := address.NewIDAddress(uint64(allocKey.Client))
	require.NoError(t, err)

	ddoPiece := func(key *miner.VerifiedAllocationKey) SafeSectorPiece {
		return SafePiece(lapi.SectorPiece{
			Piece: abi.PieceInfo{Size: pieceSize, PieceCID: pieceCid},
			DealInfo: &piece.PieceDealInfo{
				PieceActivationManifest: &miner.PieceActivationManifest{
					CID:                   pieceCid,
					Size:                  pieceSize,
					VerifiedAllocationKey: key,
				},
			},
		})
	}

	fullWeight := big.Mul(big.NewInt(int64(ssize)), big.NewInt(int64(expiration-height)))

	type allocLookup struct {
		alloc *verifreg.Allocation
		err   error
	}

	for _, tc := range []struct {
		name        string
		nv          network.Version
		piece       SafeSectorPiece
		lookup      *allocLookup
		flags       miner.SectorOnChainInfoFlags
		weight      abi.DealWeight
		recorded    int64
		pledge      int64 // what the actor charges; zero when no estimate is expected
		expectDelta int64 // the charge above the recorded pledge, before headroom
		expectSize  uint64
	}{
		{
			name:  "nv28 verified piece pledges its claimed space",
			nv:    network.Version28,
			piece: ddoPiece(allocKey), lookup: &allocLookup{alloc: &verifreg.Allocation{Client: allocKey.Client}},
			recorded: 1_000, pledge: 3_463, expectSize: uint64(pieceSize), expectDelta: 2_463,
		},
		{
			name:  "nv28 allocation not found pledges the piece at 1x",
			nv:    network.Version28,
			piece: ddoPiece(allocKey), lookup: &allocLookup{},
			recorded: 1_000, pledge: 1_021, expectSize: 0, expectDelta: 21,
		},
		{
			name:  "nv28 allocation lookup failure pledges the piece as verified",
			nv:    network.Version28,
			piece: ddoPiece(allocKey), lookup: &allocLookup{err: errors.New("no allocation for you")},
			recorded: 1_000, pledge: 3_463, expectSize: uint64(pieceSize), expectDelta: 2_463,
		},
		{
			name:     "nv28 unverified piece pledges at 1x",
			nv:       network.Version28,
			piece:    ddoPiece(nil),
			recorded: 1_000, pledge: 1_021, expectSize: 0, expectDelta: 21,
		},
		{
			name:     "nv28 recorded pledge above the requirement stands",
			nv:       network.Version28,
			piece:    ddoPiece(nil),
			recorded: 1_500, pledge: 1_021, expectSize: 0, expectDelta: 0,
		},
		{
			name:     "nv29 1x sector rises to the full-power pledge",
			nv:       network.Version29,
			piece:    ddoPiece(allocKey),
			recorded: 1_000, pledge: 10_007, expectSize: uint64(ssize), expectDelta: 9_007,
		},
		{
			name:     "nv29 recorded pledge above the full-power requirement stands",
			nv:       network.Version29,
			piece:    ddoPiece(nil),
			recorded: 12_000, pledge: 10_007, expectSize: uint64(ssize), expectDelta: 0,
		},
		{
			name:     "nv29 FULL_QA_POWER sector pledges nothing",
			nv:       network.Version29,
			piece:    ddoPiece(allocKey),
			flags:    miner.FULL_QA_POWER,
			recorded: 1_000, expectDelta: 0,
		},
		{
			name:     "nv29 legacy fully verified sector pledges nothing",
			nv:       network.Version29,
			piece:    ddoPiece(nil),
			weight:   fullWeight,
			recorded: 1_000, expectDelta: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockSealingAPI(ctrl)

			ts := makeTestTipSet(t, height)
			api.EXPECT().StateNetworkVersion(gomock.Any(), ts.Key()).Return(tc.nv, nil).AnyTimes()

			if tc.lookup != nil {
				api.EXPECT().StateGetAllocation(gomock.Any(), allocClient, verifregtypes.AllocationId(allocKey.ID), ts.Key()).
					Return(tc.lookup.alloc, tc.lookup.err)
			}
			if tc.pledge != 0 {
				// the API reports 110% of the charge, floored
				estimate := big.Div(big.Mul(big.NewInt(tc.pledge), big.NewInt(110)), big.NewInt(100))
				api.EXPECT().StateMinerInitialPledgeForSector(
					gomock.Any(), expiration-height, ssize, tc.expectSize, ts.Key(),
				).Return(estimate, nil)
			}

			weight := tc.weight
			if weight.Nil() {
				weight = big.Zero()
			}
			onChainInfo := &miner.SectorOnChainInfo{
				SectorNumber:       sectorNumber,
				SealProof:          sealProof,
				Expiration:         expiration,
				PowerBaseEpoch:     height,
				Flags:              tc.flags,
				VerifiedDealWeight: weight,
				InitialPledge:      big.NewInt(tc.recorded),
			}
			sector := SectorInfo{
				SectorNumber: sectorNumber,
				SectorType:   sealProof,
				CCUpdate:     true,
				Pieces:       []SafeSectorPiece{tc.piece},
			}

			m := &Sealing{Api: api, maddr: maddr}
			delta, err := m.replicaUpdatePledge(context.Background(), sector, onChainInfo, ts)
			require.NoError(t, err)
			// 10% headroom on the delta, floored
			expect := big.Div(big.Mul(big.NewInt(tc.expectDelta), big.NewInt(110)), big.NewInt(100))
			require.Equal(t, expect.String(), delta.String())
		})
	}
}
