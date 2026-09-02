package sealing

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
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

func makeTestTipSet(t *testing.T, height abi.ChainEpoch) *types.TipSet {
	t.Helper()

	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	dummyAddr, err := address.NewIDAddress(0)
	require.NoError(t, err)

	ts, err := types.NewTipSet([]*types.BlockHeader{{
		Height:                height,
		Miner:                 dummyAddr,
		Parents:               []cid.Cid{},
		Ticket:                &types.Ticket{VRFProof: []byte{byte(height % 2)}},
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		ParentBaseFee:         big.Zero(),
	}})
	require.NoError(t, err)

	return ts
}
