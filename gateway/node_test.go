package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	v1mocks "github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/api/v2api/v2mocks"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestGatewayAPIChainGetTipSetByHeight(t *testing.T) {
	ctx := context.Background()

	lookbackTimestamp := uint64(time.Now().Unix()) - uint64(DefaultMaxLookbackDuration.Seconds())
	type args struct {
		h         abi.ChainEpoch
		tskh      abi.ChainEpoch
		genesisTS uint64
	}
	tests := []struct {
		name   string
		args   args
		expErr bool
	}{{
		name: "basic",
		args: args{
			h:    abi.ChainEpoch(1),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "genesis",
		args: args{
			h:    abi.ChainEpoch(0),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "same epoch as tipset",
		args: args{
			h:    abi.ChainEpoch(5),
			tskh: abi.ChainEpoch(5),
		},
	}, {
		name: "tipset too old",
		args: args{
			// Tipset height is 5, genesis is at LookbackCap - 10 epochs.
			// So resulting tipset height will be 5 epochs earlier than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp - buildconstants.BlockDelaySecs*10,
		},
		expErr: true,
	}, {
		name: "lookup height too old",
		args: args{
			// Tipset height is 5, lookup height is 1, genesis is at LookbackCap - 3 epochs.
			// So
			// - lookup height will be 2 epochs earlier than LookbackCap.
			// - tipset height will be 2 epochs later than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp - buildconstants.BlockDelaySecs*3,
		},
		expErr: true,
	}, {
		name: "tipset and lookup height within acceptable range",
		args: args{
			// Tipset height is 5, lookup height is 1, genesis is at LookbackCap.
			// So
			// - lookup height will be 1 epoch later than LookbackCap.
			// - tipset height will be 5 epochs later than LookbackCap.
			h:         abi.ChainEpoch(1),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp,
		},
	}}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			defer ctrl.Finish()

			a := NewNode(mockV1, mockV2)

			// Create tipsets from genesis up to tskh and return the highest
			tss := generateTipSets(tt.args.tskh, tt.args.genesisTS)
			key := tss[len(tss)-1].Key()
			gomock.InAnyOrder(
				mockV1.EXPECT().ChainGetTipSetByHeight(gomock.AssignableToTypeOf(ctx), tt.args.h, key).DoAndReturn(
					func(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
						return tss[h], nil
					}).AnyTimes(),
			)
			gomock.InAnyOrder(
				mockV1.EXPECT().ChainGetTipSet(gomock.AssignableToTypeOf(ctx), key).DoAndReturn(
					func(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
						for _, ts := range tss {
							if ts.Key() == tsk {
								return ts, nil
							}
						}
						return nil, nil
					}).AnyTimes(),
			)
			got, err := a.v1Proxy.ChainGetTipSetByHeight(ctx, tt.args.h, key)
			if tt.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.args.h, got.Height())
			}
		})
	}
}

func generateTipSets(h abi.ChainEpoch, genesisTimestamp uint64) []*types.TipSet {
	targeth := h + 1 // add one for genesis block
	tipsets := make([]*types.TipSet, 0, targeth)
	if genesisTimestamp == 0 {
		genesisTimestamp = uint64(time.Now().Unix()) - buildconstants.BlockDelaySecs*uint64(targeth)
	}
	var currts *types.TipSet
	for currh := abi.ChainEpoch(0); currh < targeth; currh++ {
		blks := mock.MkBlock(currts, 1, 1)
		if currh == 0 {
			blks.Timestamp = genesisTimestamp
		}
		currts = mock.TipSet(blks)
		tipsets = append(tipsets, currts)
	}
	return tipsets
}

func TestGatewayVersion(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockV1 := v1mocks.NewMockFullNode(ctrl)
	mockV2 := v2mocks.NewMockFullNode(ctrl)
	defer ctrl.Finish()
	a := NewNode(mockV1, mockV2)

	mockV1.EXPECT().Version(gomock.AssignableToTypeOf(ctx)).Return(api.APIVersion{
		APIVersion: api.FullAPIVersion1,
	}, nil)

	v, err := a.v1Proxy.Version(ctx)
	require.NoError(t, err)
	require.Equal(t, api.FullAPIVersion1, v.APIVersion)
}

func TestGatewayLimitTokensAvailable(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockV1 := v1mocks.NewMockFullNode(ctrl)
	mockV2 := v2mocks.NewMockFullNode(ctrl)
	defer ctrl.Finish()
	tokens := 3
	a := NewNode(mockV1, mockV2, WithRateLimit(tokens))
	require.NoError(t, a.limit(ctx, tokens), "requests should not be limited when there are enough tokens available")
}

func TestGatewayLimitTokensRate(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockV1 := v1mocks.NewMockFullNode(ctrl)
	mockV2 := v2mocks.NewMockFullNode(ctrl)
	defer ctrl.Finish()
	tokens := 3
	rateLimit := 200
	rateLimitTimeout := time.Second / time.Duration(rateLimit/3) // large enough to not be hit
	a := NewNode(mockV1, mockV2, WithRateLimit(rateLimit), WithRateLimitTimeout(rateLimitTimeout))

	start := time.Now()
	calls := 10
	for i := 0; i < calls; i++ {
		require.NoError(t, a.limit(ctx, tokens))
	}
	// We should be slowed down by the rate limit, but not hard limited because the timeout is
	// large; the duration should be roughly the rate limit (per second) times the number of calls,
	// with one extra free call because the first one can use up the burst tokens. We'll also add a
	// couple more to account for slow test runs.
	delayPerToken := time.Second / time.Duration(rateLimit)
	expectedDuration := delayPerToken * time.Duration((calls-1)*tokens)
	expectedEnd := start.Add(expectedDuration)
	require.WithinDuration(t, expectedEnd, time.Now(), delayPerToken*time.Duration(2*tokens), "API calls should be rate limited when they hit limits")

	// In this case our timeout is too short to allow for the rate limit, so we should hit the
	// hard rate limit.
	rateLimitTimeout = time.Second / time.Duration(rateLimit)
	a = NewNode(mockV1, mockV2, WithRateLimit(rateLimit), WithRateLimitTimeout(rateLimitTimeout))
	require.NoError(t, a.limit(ctx, tokens))
	require.ErrorContains(t, a.limit(ctx, tokens), "server busy", "API calls should be hard rate limited when they hit limits")
}
