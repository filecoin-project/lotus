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

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

// From nv29, FIP-0118 gives every sector maximum quality-adjusted power regardless of its deal
// content, and StateMinerInitialPledgeForSector expresses that as a fully verified sector. The
// pieces themselves carry no verified allocation keys at that network version, so summing them
// would ask for a 1x pledge against a 10x charge and send ProveCommit out underfunded.
func TestGetSectorCollateralVerifiedSize(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(42)
	const height = abi.ChainEpoch(100)
	const expiration = abi.ChainEpoch(1000)

	sealProof := abi.RegisteredSealProof_StackedDrg32GiBV1_1
	ssize, err := sealProof.SectorSize()
	require.NoError(t, err)

	halfGiB := abi.PaddedPieceSize(512 << 20)
	verifiedPieces := []miner.PieceActivationManifest{
		{Size: halfGiB, VerifiedAllocationKey: &miner.VerifiedAllocationKey{Client: 1000, ID: 1}},
		{Size: halfGiB, VerifiedAllocationKey: &miner.VerifiedAllocationKey{Client: 1000, ID: 2}},
		{Size: halfGiB}, // unverified, never counted
	}
	unverifiedPieces := []miner.PieceActivationManifest{{Size: halfGiB}, {Size: halfGiB}}

	for _, tc := range []struct {
		name               string
		nv                 network.Version
		pieces             []miner.PieceActivationManifest
		expectVerifiedSize uint64
	}{
		{"nv28 sums the verified pieces", network.Version28, verifiedPieces, uint64(halfGiB) * 2},
		{"nv28 with unverified pieces only", network.Version28, unverifiedPieces, 0},
		{"nv28 with no pieces", network.Version28, nil, 0},
		{"nv29 ignores the verified pieces", network.Version29, verifiedPieces, uint64(ssize)},
		{"nv29 with unverified pieces only", network.Version29, unverifiedPieces, uint64(ssize)},
		{"nv29 with no pieces", network.Version29, nil, uint64(ssize)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockCommitBatcherApi(ctrl)

			ts := makeTestTipSet(t, height)
			pledge := big.NewInt(1000)
			deposit := big.NewInt(100)

			api.EXPECT().StateSectorPreCommitInfo(gomock.Any(), maddr, sectorNumber, ts.Key()).Return(
				&miner.SectorPreCommitOnChainInfo{
					Info:             miner.SectorPreCommitInfo{SealProof: sealProof, Expiration: expiration},
					PreCommitDeposit: deposit,
				}, nil)
			api.EXPECT().StateNetworkVersion(gomock.Any(), ts.Key()).Return(tc.nv, nil)
			api.EXPECT().StateMinerInitialPledgeForSector(
				gomock.Any(),
				gomock.Eq(expiration-height),
				gomock.Eq(ssize),
				gomock.Eq(tc.expectVerifiedSize),
				gomock.Eq(ts.Key()),
			).Return(pledge, nil)

			b := &CommitBatcher{api: api, maddr: maddr, mctx: context.Background()}

			collateral, err := b.getSectorCollateral(sectorNumber, tc.pieces, ts)
			require.NoError(t, err)
			require.Equal(t, big.Sub(pledge, deposit), collateral)
		})
	}
}

// From nv29 (FIP-0118) the miner actor ignores verified_allocation_key, so ProveCommit shouldn't
// gate on verifreg.
func TestProcessBatchV2AllocationCheck(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(42)
	const height = abi.ChainEpoch(100)
	const expiration = abi.ChainEpoch(1000)

	sealProof := abi.RegisteredSealProof_StackedDrg32GiBV1_1
	ssize, err := sealProof.SectorSize()
	require.NoError(t, err)

	pieceCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	allocKey := &miner.VerifiedAllocationKey{Client: 1000, ID: 1}
	client, err := address.NewIDAddress(uint64(allocKey.Client))
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		nv   network.Version
	}{
		{"nv28 checks the allocation", network.Version28},
		{"nv29 skips the allocation", network.Version29},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockCommitBatcherApi(ctrl)

			ts := makeTestTipSet(t, height)
			api.EXPECT().ChainHead(gomock.Any()).AnyTimes().Return(ts, nil)
			api.EXPECT().StateSectorPreCommitInfo(gomock.Any(), maddr, sectorNumber, ts.Key()).AnyTimes().Return(
				&miner.SectorPreCommitOnChainInfo{
					Info:             miner.SectorPreCommitInfo{SealProof: sealProof, Expiration: expiration},
					PreCommitDeposit: big.Zero(),
				}, nil)
			api.EXPECT().StateNetworkVersion(gomock.Any(), ts.Key()).AnyTimes().Return(tc.nv, nil)
			api.EXPECT().StateMinerInitialPledgeForSector(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(big.NewInt(1000), nil)

			b := &CommitBatcher{
				api:   api,
				maddr: maddr,
				mctx:  context.Background(),
				todo: map[abi.SectorNumber]AggregateInput{sectorNumber: {
					Spt: sealProof,
					ActivationManifest: miner.SectorActivationManifest{
						SectorNumber: sectorNumber,
						Pieces: []miner.PieceActivationManifest{{
							CID:                   pieceCid,
							Size:                  abi.PaddedPieceSize(ssize),
							VerifiedAllocationKey: allocKey,
						}},
					},
				}},
			}

			cfg := sealiface.Config{CollateralFromMinerBalance: true}

			if tc.nv < network.Version29 {
				// No allocation on chain, so the sector fails the check and the batch empties out.
				api.EXPECT().StateGetAllocation(gomock.Any(), client, verifregtypes.AllocationId(allocKey.ID), ts.Key()).
					Return(nil, nil)

				res, err := b.processBatchV2(cfg, []abi.SectorNumber{sectorNumber}, tc.nv, false)
				require.NoError(t, err)
				require.Nil(t, res)
				return
			}

			// Past the check the batch runs on; stop it at the next call out.
			balanceErr := errors.New("no balance for you")
			api.EXPECT().StateMinerAvailableBalance(gomock.Any(), maddr, types.EmptyTSK).Return(big.Zero(), balanceErr)

			res, err := b.processBatchV2(cfg, []abi.SectorNumber{sectorNumber}, tc.nv, false)
			require.ErrorIs(t, err, balanceErr)
			require.Len(t, res, 1)
			require.Empty(t, res[0].FailedSectors)
			require.Equal(t, []abi.SectorNumber{sectorNumber}, res[0].Sectors)
		})
	}
}
