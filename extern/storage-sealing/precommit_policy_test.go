package sealing_test

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/network"
	api "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
)

type fakeChain struct {
	h abi.ChainEpoch
}

type fakeConfigStub struct {
	CCSectorLifetime time.Duration
}

func fakeConfigGetter(stub *fakeConfigStub) sealing.GetSealingConfigFunc {
	return func() (sealiface.Config, error) {
		if stub == nil {
			return sealiface.Config{}, nil
		}

		return sealiface.Config{
			CommittedCapacitySectorLifetime: stub.CCSectorLifetime,
		}, nil
	}
}

func (f *fakeChain) StateNetworkVersion(ctx context.Context, tok sealing.TipSetToken) (network.Version, error) {
	return build.NewestNetworkVersion, nil
}

func (f *fakeChain) ChainHead(ctx context.Context) (sealing.TipSetToken, abi.ChainEpoch, error) {
	return []byte{1, 2, 3}, f.h, nil
}

func fakePieceCid(t *testing.T) cid.Cid {
	comm := [32]byte{1, 2, 3}
	fakePieceCid, err := commcid.ReplicaCommitmentV1ToCID(comm[:])
	require.NoError(t, err)
	return fakePieceCid
}

func TestBasicPolicyEmptySector(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	h := abi.ChainEpoch(55)
	pBoundary := abi.ChainEpoch(0)
	pBuffer := abi.ChainEpoch(2)
	pcp := sealing.NewBasicPreCommitPolicy(&fakeChain{h: h}, cfg, pBoundary, pBuffer)
	exp, err := pcp.Expiration(context.Background())

	require.NoError(t, err)

	// as set when there are no deal pieces
	expected := h + policy.GetMaxSectorExpirationExtension() - (pBuffer * 2)
	// as set just before returning within Expiration()
	expected += miner.WPoStProvingPeriod - (expected % miner.WPoStProvingPeriod) + pBoundary - 1
	assert.Equal(t, int(expected), int(exp))
}

func TestCustomCCSectorConfig(t *testing.T) {
	customLifetime := 200 * 24 * time.Hour
	customLifetimeEpochs := abi.ChainEpoch(int64(customLifetime.Seconds()) / builtin.EpochDurationSeconds)
	cfgStub := fakeConfigStub{CCSectorLifetime: customLifetime}
	cfg := fakeConfigGetter(&cfgStub)
	h := abi.ChainEpoch(55)
	pBoundary := abi.ChainEpoch(0)
	pBuffer := abi.ChainEpoch(2)
	pcp := sealing.NewBasicPreCommitPolicy(&fakeChain{h: h}, cfg, pBoundary, pBuffer)
	exp, err := pcp.Expiration(context.Background())

	require.NoError(t, err)

	// as set when there are no deal pieces
	expected := h + customLifetimeEpochs - (pBuffer * 2)
	// as set just before returning within Expiration()
	expected += miner.WPoStProvingPeriod - (expected % miner.WPoStProvingPeriod) + pBoundary - 1
	assert.Equal(t, int(expected), int(exp))
}

func TestBasicPolicyMostConstrictiveSchedule(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	pPeriod := abi.ChainEpoch(11)
	policy := sealing.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, pPeriod, 2)
	longestDealEpochEnd := abi.ChainEpoch(100)
	pieces := []sealing.Piece{
		{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &api.PieceDealInfo{
				DealID: abi.DealID(42),
				DealSchedule: api.DealSchedule{
					StartEpoch: abi.ChainEpoch(70),
					EndEpoch:   abi.ChainEpoch(75),
				},
			},
		},
		{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &api.PieceDealInfo{
				DealID: abi.DealID(43),
				DealSchedule: api.DealSchedule{
					StartEpoch: abi.ChainEpoch(80),
					EndEpoch:   longestDealEpochEnd,
				},
			},
		},
	}

	exp, err := policy.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	expected := longestDealEpochEnd + miner.WPoStProvingPeriod - (longestDealEpochEnd % miner.WPoStProvingPeriod) + pPeriod - 1
	assert.Equal(t, int(expected), int(exp))
}

func TestBasicPolicyIgnoresExistingScheduleIfExpired(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	policy := sealing.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 0, 0)

	pieces := []sealing.Piece{
		{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &api.PieceDealInfo{
				DealID: abi.DealID(44),
				DealSchedule: api.DealSchedule{
					StartEpoch: abi.ChainEpoch(1),
					EndEpoch:   abi.ChainEpoch(10),
				},
			},
		},
	}

	exp, err := policy.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	assert.Equal(t, 1558079, int(exp))
}

func TestMissingDealIsIgnored(t *testing.T) {
	cfg := fakeConfigGetter(nil)
	policy := sealing.NewBasicPreCommitPolicy(&fakeChain{
		h: abi.ChainEpoch(55),
	}, cfg, 11, 0)

	pieces := []sealing.Piece{
		{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: &api.PieceDealInfo{
				DealID: abi.DealID(44),
				DealSchedule: api.DealSchedule{
					StartEpoch: abi.ChainEpoch(1),
					EndEpoch:   abi.ChainEpoch(10),
				},
			},
		},
		{
			Piece: abi.PieceInfo{
				Size:     abi.PaddedPieceSize(1024),
				PieceCID: fakePieceCid(t),
			},
			DealInfo: nil,
		},
	}

	exp, err := policy.Expiration(context.Background(), pieces...)
	require.NoError(t, err)

	assert.Equal(t, 1558090, int(exp))
}
