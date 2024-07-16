package sealing_test

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	pipeline "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

type fakeChain struct {
	h abi.ChainEpoch
}

type fakeConfigStub struct {
	CCSectorLifetime time.Duration
}

func fakeConfigGetter(stub *fakeConfigStub) dtypes.GetSealingConfigFunc {
	return func() (sealiface.Config, error) {
		if stub == nil {
			return sealiface.Config{}, nil
		}

		return sealiface.Config{
			CommittedCapacitySectorLifetime: stub.CCSectorLifetime,
		}, nil
	}
}

func (f *fakeChain) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	return buildconstants.TestNetworkVersion, nil
}

func makeBFTs(t *testing.T, basefee abi.TokenAmount, h abi.ChainEpoch) *types.TipSet {
	dummyCid, _ := cid.Parse("bafkqaaa")

	var ts, err = types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  builtin.SystemActorAddr,

			Parents: []cid.Cid{},

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              dummyCid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

			ParentBaseFee: basefee,
		},
	})
	if t != nil {
		require.NoError(t, err)
	}

	return ts
}

func makeTs(t *testing.T, h abi.ChainEpoch) *types.TipSet {
	return makeBFTs(t, big.NewInt(0), h)
}

func (f *fakeChain) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return makeTs(nil, f.h), nil
}

func fakePieceCid(t *testing.T) cid.Cid {
	comm := [32]byte{1, 2, 3}
	fakePieceCid, err := commcid.ReplicaCommitmentV1ToCID(comm[:])
	require.NoError(t, err)
	return fakePieceCid
}

func cidPtr(c cid.Cid) *cid.Cid {
	return &c
}

func TestBasicPolicyEmptySector(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	h := abi.ChainEpoch(55)
	pBuffer := abi.ChainEpoch(2)
	pcp := pipeline.NewBasicPreCommitPolicy(&fakeChain{h: h}, cfg, pBuffer)
	exp, err := pcp.Expiration(context.Background())

	require.NoError(t, err)

	// as set when there are no deal pieces
	maxExtension, err := policy.GetMaxSectorExpirationExtension(buildconstants.TestNetworkVersion)
	assert.NoError(t, err)
	expected := h + maxExtension - pBuffer
	assert.Equal(t, int(expected), int(exp))
}

func TestCustomCCSectorConfig(t *testing.T) {
	customLifetime := 200 * 24 * time.Hour
	customLifetimeEpochs := abi.ChainEpoch(int64(customLifetime.Seconds()) / builtin.EpochDurationSeconds)
	cfgStub := fakeConfigStub{CCSectorLifetime: customLifetime}
	cfg := fakeConfigGetter(&cfgStub)
	h := abi.ChainEpoch(55)
	pBuffer := abi.ChainEpoch(2)
	pcp := pipeline.NewBasicPreCommitPolicy(&fakeChain{h: h}, cfg, pBuffer)
	exp, err := pcp.Expiration(context.Background())

	require.NoError(t, err)

	// as set when there are no deal pieces
	expected := h + customLifetimeEpochs - pBuffer
	assert.Equal(t, int(expected), int(exp))
}

func TestBasicPolicyMostConstrictiveSchedule(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	policy := pipeline.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 2)
	longestDealEpochEnd := abi.ChainEpoch(547300)
	pieces := []pipeline.SafeSectorPiece{
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &piece.PieceDealInfo{
				PublishCid: cidPtr(fakePieceCid(t)), // pretend this is a valid builtin-market deal
				DealID:     abi.DealID(42),
				DealSchedule: piece.DealSchedule{
					StartEpoch: abi.ChainEpoch(70),
					EndEpoch:   abi.ChainEpoch(547275),
				},
			},
		}),
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &piece.PieceDealInfo{
				PublishCid: cidPtr(fakePieceCid(t)), // pretend this is a valid builtin-market deal
				DealID:     abi.DealID(43),
				DealSchedule: piece.DealSchedule{
					StartEpoch: abi.ChainEpoch(80),
					EndEpoch:   longestDealEpochEnd,
				},
			},
		}),
	}

	exp, err := policy.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	assert.Equal(t, int(longestDealEpochEnd), int(exp))
}

func TestBasicPolicyIgnoresExistingScheduleIfExpired(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	pcp := pipeline.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 0)

	pieces := []pipeline.SafeSectorPiece{
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &piece.PieceDealInfo{
				PublishCid: cidPtr(fakePieceCid(t)), // pretend this is a valid builtin-market deal
				DealID:     abi.DealID(44),
				DealSchedule: piece.DealSchedule{
					StartEpoch: abi.ChainEpoch(1),
					EndEpoch:   abi.ChainEpoch(10),
				},
			},
		}),
	}

	exp, err := pcp.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	maxLifetime, err := policy.GetMaxSectorExpirationExtension(buildconstants.TestNetworkVersion)
	require.NoError(t, err)

	// Treated as a CC sector, so expiration becomes currEpoch + maxLifetime = 55 + 1555200
	assert.Equal(t, 55+maxLifetime, exp)
}

func TestMissingDealIsIgnored(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	policy := pipeline.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 0)

	pieces := []pipeline.SafeSectorPiece{
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &piece.PieceDealInfo{
				PublishCid: cidPtr(fakePieceCid(t)), // pretend this is a valid builtin-market deal
				DealID:     abi.DealID(44),
				DealSchedule: piece.DealSchedule{
					StartEpoch: abi.ChainEpoch(1),
					EndEpoch:   abi.ChainEpoch(547300),
				},
			},
		}),
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: nil,
		}),
	}

	exp, err := policy.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	assert.Equal(t, 547300, int(exp))
}

func TestBasicPolicyDDO(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	pcp := pipeline.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 0)

	pieces := []pipeline.SafeSectorPiece{
		pipeline.SafePiece(api.SectorPiece{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &piece.PieceDealInfo{
				PublishCid: nil,
				DealID:     abi.DealID(44),
				DealSchedule: piece.DealSchedule{
					StartEpoch: abi.ChainEpoch(100_000),
					EndEpoch:   abi.ChainEpoch(1500_000),
				},
				PieceActivationManifest: &miner.PieceActivationManifest{
					Size:                  0,
					VerifiedAllocationKey: nil,
					Notify:                nil,
				},
			},
		}),
	}

	exp, err := pcp.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	assert.Equal(t, abi.ChainEpoch(1500_000), exp)
}
