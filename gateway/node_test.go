package gateway

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	v1mocks "github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/api/v2api/v2mocks"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
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
		name     string
		args     args
		noParams bool // rejected before any epoch time is needed
		expErr   string
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
		expErr: "lookbacks of more than",
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
		expErr: "lookbacks of more than",
	}, {
		name: "height in future",
		args: args{
			// Genesis is 6 epochs ago, so height 10 is 4 epochs in the future,
			// but it is above the anchor first.
			h:    abi.ChainEpoch(10),
			tskh: abi.ChainEpoch(5),
		},
		noParams: true,
		expErr:   "is above the anchor tipset at 5",
	}, {
		name: "height above the keyed tipset",
		args: args{
			// Height 10 is in the past by the clock but above the key's height 5.
			h:         abi.ChainEpoch(10),
			tskh:      abi.ChainEpoch(5),
			genesisTS: lookbackTimestamp,
		},
		noParams: true,
		expErr:   "height 10 is above the anchor tipset at 5",
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
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			defer ctrl.Finish()

			a := NewNode(mockV1, mockV2)

			// Create tipsets from genesis up to tskh and return the highest
			tss := generateTipSets(tt.args.tskh, tt.args.genesisTS)
			key := tss[len(tss)-1].Key()
			params := expectNetworkParams(mockV1, tss[0].MinTimestamp())
			if tt.noParams {
				params.Times(0)
			}
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
					}).Times(1),
			)
			got, err := a.v1Proxy.ChainGetTipSetByHeight(ctx, tt.args.h, key)
			if tt.expErr != "" {
				require.ErrorContains(t, err, tt.expErr)
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

// expectNetworkParams expects exactly one network params fetch, so a test using
// it also proves the memo holds within a Node.
func expectNetworkParams(mockV1 *v1mocks.MockFullNode, genesisTS uint64) *gomock.Call {
	return mockV1.EXPECT().StateGetNetworkParams(gomock.Any()).Return(&api.NetworkParams{
		GenesisTimestamp: genesisTS,
		BlockDelaySecs:   buildconstants.BlockDelaySecs,
	}, nil).Times(1)
}

func TestV2GatewayTipSetSelectorLookback(t *testing.T) {
	ctx := context.Background()

	// Tipsets below height 10 are older than the lookback bound; 11 and above
	// are within it.
	lookbackTimestamp := uint64(time.Now().Unix()) - uint64(DefaultMaxLookbackDuration.Seconds())
	tss := generateTipSets(15, lookbackTimestamp-buildconstants.BlockDelaySecs*10)
	genesisTS := tss[0].MinTimestamp()
	oldTs, recentTs := tss[1], tss[13]

	addr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	invalid := types.TipSetSelectors.Key(recentTs.Key())
	invalid.Tag = &types.TipSetTags.Latest

	newNode := func(t *testing.T, checked bool) (*Node, *v1mocks.MockFullNode, *v2mocks.MockFullNode) {
		ctrl := gomock.NewController(t)
		mockV1 := v1mocks.NewMockFullNode(ctrl)
		mockV2 := v2mocks.NewMockFullNode(ctrl)
		if checked {
			expectNetworkParams(mockV1, genesisTS)
		}
		return NewNode(mockV1, mockV2), mockV1, mockV2
	}

	t.Run("StateGetActor", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			selector  types.TipSetSelector
			checked   bool          // the lookback check runs
			byKey     *types.TipSet // v1 ChainGetTipSet result for the selector's key
			byTag     *types.TipSet // v2 ChainGetTipSet result for the selector's tag
			forwarded bool
			lookback  bool
		}{
			{name: "latest", selector: types.TipSetSelectors.Latest, forwarded: true},
			{name: "key in bound", selector: types.TipSetSelectors.Key(recentTs.Key()), checked: true, byKey: recentTs, forwarded: true},
			{name: "key too old", selector: types.TipSetSelectors.Key(oldTs.Key()), checked: true, byKey: oldTs, lookback: true},
			{name: "height in bound", selector: types.TipSetSelectors.Height(recentTs.Height(), false, nil), checked: true, forwarded: true},
			{name: "height too old", selector: types.TipSetSelectors.Height(oldTs.Height(), false, nil), checked: true, lookback: true},
			{name: "finalized in bound", selector: types.TipSetSelectors.Finalized, checked: true, byTag: recentTs, forwarded: true},
			{name: "finalized too old", selector: types.TipSetSelectors.Finalized, checked: true, byTag: oldTs, lookback: true},
			{name: "invalid", selector: invalid},
		} {
			t.Run(tc.name, func(t *testing.T) {
				a, mockV1, mockV2 := newNode(t, tc.checked)
				if tc.byKey != nil {
					mockV1.EXPECT().ChainGetTipSet(gomock.Any(), tc.byKey.Key()).Return(tc.byKey, nil)
				}
				if tc.byTag != nil {
					mockV2.EXPECT().ChainGetTipSet(gomock.Any(), gomock.Eq(tc.selector)).Return(tc.byTag, nil)
				}
				if tc.forwarded {
					mockV2.EXPECT().StateGetActor(gomock.Any(), addr, gomock.Eq(tc.selector)).Return(&types.Actor{}, nil)
				}

				_, err := a.v2Proxy.StateGetActor(ctx, addr, tc.selector)
				switch {
				case tc.lookback:
					require.ErrorIs(t, err, a.errLookback)
				case !tc.forwarded:
					require.ErrorContains(t, err, "validating selector")
				default:
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("StateGetID", func(t *testing.T) {
		a, mockV1, _ := newNode(t, true)
		mockV1.EXPECT().ChainGetTipSet(gomock.Any(), oldTs.Key()).Return(oldTs, nil)
		_, err := a.v2Proxy.StateGetID(ctx, addr, types.TipSetSelectors.Key(oldTs.Key()))
		require.ErrorIs(t, err, a.errLookback)
	})

	t.Run("StateRewardDistribution", func(t *testing.T) {
		a, mockV1, _ := newNode(t, true)
		mockV1.EXPECT().ChainGetTipSet(gomock.Any(), oldTs.Key()).Return(oldTs, nil)
		_, err := a.v2Proxy.StateRewardDistribution(ctx, types.TipSetSelectors.Key(oldTs.Key()))
		require.ErrorIs(t, err, a.errLookback)
	})

	t.Run("ChainGetTipSet key passes through", func(t *testing.T) {
		a, _, mockV2 := newNode(t, false)
		sel := types.TipSetSelectors.Key(oldTs.Key())
		mockV2.EXPECT().ChainGetTipSet(gomock.Any(), gomock.Eq(sel)).Return(oldTs, nil)
		got, err := a.v2Proxy.ChainGetTipSet(ctx, sel)
		require.NoError(t, err)
		require.Equal(t, oldTs, got)
	})

	t.Run("ChainGetTipSet height in bound", func(t *testing.T) {
		a, _, mockV2 := newNode(t, true)
		sel := types.TipSetSelectors.Height(recentTs.Height(), false, nil)
		mockV2.EXPECT().ChainGetTipSet(gomock.Any(), gomock.Eq(sel)).Return(recentTs, nil)
		got, err := a.v2Proxy.ChainGetTipSet(ctx, sel)
		require.NoError(t, err)
		require.Equal(t, recentTs, got)
	})

	t.Run("ChainGetTipSet height too old", func(t *testing.T) {
		a, _, _ := newNode(t, true)
		_, err := a.v2Proxy.ChainGetTipSet(ctx, types.TipSetSelectors.Height(oldTs.Height(), false, nil))
		require.ErrorIs(t, err, a.errLookback)
	})
}

func TestGatewayLookbackMemo(t *testing.T) {
	ctx := context.Background()
	tss := generateTipSets(5, 0)
	genesisTS := tss[0].MinTimestamp()
	ts := tss[len(tss)-1]

	newNode := func(t *testing.T) (*Node, *v1mocks.MockFullNode) {
		ctrl := gomock.NewController(t)
		mockV1 := v1mocks.NewMockFullNode(ctrl)
		return NewNode(mockV1, v2mocks.NewMockFullNode(ctrl)), mockV1
	}

	t.Run("params fetched once", func(t *testing.T) {
		a, mockV1 := newNode(t)
		expectNetworkParams(mockV1, genesisTS)
		for h := abi.ChainEpoch(0); h < 5; h++ {
			require.NoError(t, a.checkEpoch(ctx, h))
		}
	})

	t.Run("failed params fetch retries", func(t *testing.T) {
		a, mockV1 := newNode(t)
		gomock.InOrder(
			mockV1.EXPECT().StateGetNetworkParams(gomock.Any()).Return(nil, errors.New("backend down")),
			expectNetworkParams(mockV1, genesisTS),
		)
		require.ErrorContains(t, a.checkEpoch(ctx, 1), "backend down")
		require.NoError(t, a.checkEpoch(ctx, 1))
		require.NoError(t, a.checkEpoch(ctx, 1))
	})

	t.Run("tipset height cached", func(t *testing.T) {
		a, mockV1 := newNode(t)
		expectNetworkParams(mockV1, genesisTS)
		mockV1.EXPECT().ChainGetTipSet(gomock.Any(), ts.Key()).Return(ts, nil).Times(1)
		require.NoError(t, a.checkTipSetKey(ctx, ts.Key()))
		require.NoError(t, a.checkTipSetKey(ctx, ts.Key()))
	})

	t.Run("eth hash cached", func(t *testing.T) {
		a, mockV1 := newNode(t)
		expectNetworkParams(mockV1, genesisTS)
		tskCid, err := ts.Key().Cid()
		require.NoError(t, err)
		hash, err := ethtypes.EthHashFromCid(tskCid)
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, ts.Key().MarshalCBOR(&buf))
		mockV1.EXPECT().ChainReadObj(gomock.Any(), tskCid).Return(buf.Bytes(), nil).Times(1)
		mockV1.EXPECT().ChainGetTipSet(gomock.Any(), ts.Key()).Return(ts, nil).Times(1)
		require.NoError(t, a.checkEthBlockHash(ctx, hash))
		require.NoError(t, a.checkEthBlockHash(ctx, hash))
	})
}

func TestGatewayEthBlockParamLookback(t *testing.T) {
	ctx := context.Background()

	// Height 1 is older than the default lookback bound; height 13 is within it
	// but older than a one minute bound.
	lookbackTimestamp := uint64(time.Now().Unix()) - uint64(DefaultMaxLookbackDuration.Seconds())
	tss := generateTipSets(15, lookbackTimestamp-buildconstants.BlockDelaySecs*10)
	genesisTS := tss[0].MinTimestamp()
	oldTs := tss[13]
	// A tipset a few minutes behind now, where F3 or the EC calculator place
	// "finalized" on a live chain.
	nearHead := mock.MkBlock(nil, 1, 1)
	nearHead.Height = abi.ChainEpoch(DefaultMaxLookbackDuration/(time.Duration(buildconstants.BlockDelaySecs)*time.Second)) + 5
	recentTs := mock.TipSet(nearHead)

	blockParam := func(param string) ethtypes.EthBlockNumberOrHash {
		var bp ethtypes.EthBlockNumberOrHash
		if err := bp.UnmarshalJSON([]byte(`"` + param + `"`)); err != nil {
			panic(err)
		}
		return bp
	}
	checkers := []struct {
		name  string
		check func(a *Node, param string, lookback ethtypes.EthUint64) error
	}{
		{"checkEthBlockNumber", func(a *Node, param string, lookback ethtypes.EthUint64) error {
			return a.checkEthBlockNumber(ctx, param, lookback)
		}},
		{"checkEthBlockParam", func(a *Node, param string, lookback ethtypes.EthUint64) error {
			return a.checkEthBlockParam(ctx, blockParam(param), lookback)
		}},
	}

	for _, tc := range []struct {
		name     string
		param    string
		lookback ethtypes.EthUint64
		opts     []Option
		checked  bool          // the check consults network params
		resolved *types.TipSet // what the backend returns for a tag; nil for no resolve
		wantErr  string
	}{
		{name: "latest", param: "latest"},
		{name: "old number", param: "0x1", checked: true, wantErr: "lookbacks of more than"},
		{name: "recent number", param: "0xd", checked: true},
		{name: "huge number", param: "0x10000000000", checked: true, wantErr: "tipset height in future"},
		{name: "safe", param: "safe", checked: true},
		{name: "finalized", param: "finalized", checked: true},
		{name: "finalized tight bound resolves old", param: "finalized", opts: []Option{WithMaxLookbackDuration(time.Hour)}, checked: true, resolved: oldTs, wantErr: "lookbacks of more than"},
		{name: "finalized tight bound resolves recent", param: "finalized", opts: []Option{WithMaxLookbackDuration(time.Hour)}, checked: true, resolved: recentTs},
		{name: "safe tight bound resolves recent", param: "safe", opts: []Option{WithMaxLookbackDuration(10 * time.Minute)}, checked: true, resolved: recentTs},
		{name: "latest with lookback", param: "latest", lookback: 100, checked: true},
		{name: "latest with lookback short bound", param: "latest", lookback: 100, opts: []Option{WithMaxLookbackDuration(time.Minute)}, checked: true, wantErr: "lookbacks of more than"},
	} {
		for _, c := range checkers {
			t.Run(c.name+"/"+tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				mockV1 := v1mocks.NewMockFullNode(ctrl)
				mockV2 := v2mocks.NewMockFullNode(ctrl)
				a := NewNode(mockV1, mockV2, tc.opts...)
				if tc.checked {
					expectNetworkParams(mockV1, genesisTS)
				}
				if tc.resolved != nil {
					selector := types.TipSetSelectors.Finalized
					if tc.param == "safe" {
						selector = types.TipSetSelectors.Safe
					}
					mockV2.EXPECT().ChainGetTipSet(gomock.Any(), gomock.Eq(selector)).Return(tc.resolved, nil)
				}
				err := c.check(a, tc.param, tc.lookback)
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
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

// TestGatewayEthMessageLookbackBounded verifies that both gateway proxies'
// eth message- and receipt-lookup methods forward to the backend's *Limited
// variant with the gateway's configured maxMessageLookbackEpochs. The Gateway
// interfaces themselves do not expose the *Limited variants.
func TestGatewayEthMessageLookbackBounded(t *testing.T) {
	ctx := context.Background()
	const bound = abi.ChainEpoch(7)

	txHash := ethtypes.EthHash{0xde, 0xad}
	blkParam := ethtypes.EthBlockNumberOrHash{}
	num := "latest"
	blkParam.PredefinedBlock = &num

	// Each method has a (proxy, backend-mock) pair per gateway version. The
	// expectation is set on exactly the version's backend mock; if the proxy
	// hits the wrong mock (or none), gomock reports an unexpected call.
	type variant struct {
		name   string
		expect func(v1 *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode)
		call   func(a *Node) error
	}

	cases := []struct {
		method   string
		variants []variant
	}{
		{
			method: "EthGetTransactionByHash",
			variants: []variant{
				{"v1", func(v1 *v1mocks.MockFullNode, _ *v2mocks.MockFullNode) {
					v1.EXPECT().EthGetTransactionByHashLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(&txHash), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v1Proxy.EthGetTransactionByHash(ctx, &txHash); return err }},
				{"v2", func(_ *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode) {
					v2.EXPECT().EthGetTransactionByHashLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(&txHash), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v2Proxy.EthGetTransactionByHash(ctx, &txHash); return err }},
			},
		},
		{
			method: "EthGetTransactionReceipt",
			variants: []variant{
				{"v1", func(v1 *v1mocks.MockFullNode, _ *v2mocks.MockFullNode) {
					v1.EXPECT().EthGetTransactionReceiptLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(txHash), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v1Proxy.EthGetTransactionReceipt(ctx, txHash); return err }},
				{"v2", func(_ *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode) {
					v2.EXPECT().EthGetTransactionReceiptLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(txHash), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v2Proxy.EthGetTransactionReceipt(ctx, txHash); return err }},
			},
		},
		{
			method: "EthGetBlockReceipts",
			variants: []variant{
				{"v1", func(v1 *v1mocks.MockFullNode, _ *v2mocks.MockFullNode) {
					v1.EXPECT().EthGetBlockReceiptsLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(blkParam), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v1Proxy.EthGetBlockReceipts(ctx, blkParam); return err }},
				{"v2", func(_ *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode) {
					v2.EXPECT().EthGetBlockReceiptsLimited(gomock.AssignableToTypeOf(ctx), gomock.Eq(blkParam), gomock.Eq(bound)).Return(nil, nil)
				}, func(a *Node) error { _, err := a.v2Proxy.EthGetBlockReceipts(ctx, blkParam); return err }},
			},
		},
	}

	for _, tc := range cases {
		for _, v := range tc.variants {
			t.Run(v.name+"/"+tc.method, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockV1 := v1mocks.NewMockFullNode(ctrl)
				mockV2 := v2mocks.NewMockFullNode(ctrl)
				a := NewNode(mockV1, mockV2, WithMaxMessageLookbackEpochs(bound))
				v.expect(mockV1, mockV2)
				require.NoError(t, v.call(a))
			})
		}
	}
}

// TestGatewayEthSendRawTransactionUsesUntrusted verifies that both gateway
// proxies forward EthSendRawTransaction to the backend's
// EthSendRawTransactionUntrusted (which routes through MpoolPushUntrusted).
// The Untrusted variant is not exposed on the Gateway interfaces themselves.
func TestGatewayEthSendRawTransactionUsesUntrusted(t *testing.T) {
	ctx := context.Background()
	rawTx := ethtypes.EthBytes{0x01, 0x02, 0x03}

	cases := []struct {
		version string
		expect  func(v1 *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode)
		call    func(a *Node) error
	}{
		{
			version: "v1",
			expect: func(v1 *v1mocks.MockFullNode, _ *v2mocks.MockFullNode) {
				v1.EXPECT().EthSendRawTransactionUntrusted(gomock.AssignableToTypeOf(ctx), gomock.Eq(rawTx)).Return(ethtypes.EthHash{}, nil)
			},
			call: func(a *Node) error { _, err := a.v1Proxy.EthSendRawTransaction(ctx, rawTx); return err },
		},
		{
			version: "v2",
			expect: func(_ *v1mocks.MockFullNode, v2 *v2mocks.MockFullNode) {
				v2.EXPECT().EthSendRawTransactionUntrusted(gomock.AssignableToTypeOf(ctx), gomock.Eq(rawTx)).Return(ethtypes.EthHash{}, nil)
			},
			call: func(a *Node) error { _, err := a.v2Proxy.EthSendRawTransaction(ctx, rawTx); return err },
		},
	}

	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			a := NewNode(mockV1, mockV2)
			tc.expect(mockV1, mockV2)
			require.NoError(t, tc.call(a))
		})
	}
}

// TestV1GatewayStateMessageLookbackClamped exercises the limit normalisation
// applied to StateSearchMsg and StateWaitMsg: a caller-supplied limit is
// normalised to maxMessageLookbackEpochs when it is LookbackNoLimit (-1) or
// when it exceeds the bound; a limit below the bound is forwarded unchanged.
func TestV1GatewayStateMessageLookbackClamped(t *testing.T) {
	ctx := context.Background()
	const bound = abi.ChainEpoch(10)

	cases := []struct {
		name string
		in   abi.ChainEpoch
		want abi.ChainEpoch
	}{
		{"LookbackNoLimit clamps to bound", api.LookbackNoLimit, bound},
		{"limit above bound clamps", bound + 5, bound},
		{"limit at bound passes through", bound, bound},
		{"limit below bound passes through", bound - 1, bound - 1},
	}

	emptyTSK := types.EmptyTSK
	msgCid := cid.Undef

	for _, tc := range cases {
		t.Run("StateSearchMsg/"+tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			a := NewNode(mockV1, mockV2, WithMaxMessageLookbackEpochs(bound))

			mockV1.EXPECT().
				StateSearchMsg(gomock.AssignableToTypeOf(ctx), gomock.Eq(emptyTSK), gomock.Eq(msgCid), gomock.Eq(tc.want), gomock.Eq(true)).
				Return(nil, nil)

			_, err := a.v1Proxy.StateSearchMsg(ctx, emptyTSK, msgCid, tc.in, true)
			require.NoError(t, err)
		})

		t.Run("StateWaitMsg/"+tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			a := NewNode(mockV1, mockV2, WithMaxMessageLookbackEpochs(bound))

			mockV1.EXPECT().
				StateWaitMsg(gomock.AssignableToTypeOf(ctx), gomock.Eq(msgCid), gomock.Eq(uint64(0)), gomock.Eq(tc.want), gomock.Eq(true)).
				Return(nil, nil)

			_, err := a.v1Proxy.StateWaitMsg(ctx, msgCid, 0, tc.in, true)
			require.NoError(t, err)
		})
	}
}

func TestV1GatewayStateWaitMsgConfidenceLimit(t *testing.T) {
	ctx := context.Background()
	const bound = uint64(10)
	msgCid := cid.Undef

	tests := []struct {
		name       string
		confidence uint64
		wantErr    bool
	}{
		{"zero confidence", 0, false},
		{"confidence at bound", bound, false},
		{"confidence above bound", bound + 1, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			a := NewNode(mockV1, mockV2, WithMaxMessageConfidence(bound))

			if !test.wantErr {
				mockV1.EXPECT().
					StateWaitMsg(gomock.AssignableToTypeOf(ctx), gomock.Eq(msgCid), gomock.Eq(test.confidence), gomock.Eq(DefaultMaxMessageLookbackEpochs), gomock.Eq(true)).
					Return(nil, nil)
			}

			_, err := a.v1Proxy.StateWaitMsg(ctx, msgCid, test.confidence, api.LookbackNoLimit, true)
			if test.wantErr {
				require.ErrorContains(t, err, "exceeds gateway maximum")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGatewayInterfacesDoNotExposeLimitedOrUntrusted asserts the invariant
// that neither Gateway interface exposes a `*Limited` or `*Untrusted` method
// variant. Those variants exist on FullNode for internal use by the gateway
// proxies, which apply the gateway's maxMessageLookbackEpochs and route mpool
// pushes through the Untrusted backend method.
func TestGatewayInterfacesDoNotExposeLimitedOrUntrusted(t *testing.T) {
	check := func(t *testing.T, iface reflect.Type) {
		for i := 0; i < iface.NumMethod(); i++ {
			name := iface.Method(i).Name
			require.Falsef(t, strings.HasSuffix(name, "Limited"),
				"%s.%s should not appear on the Gateway interface; *Limited variants are used internally by the gateway proxy",
				iface.Name(), name)
			require.Falsef(t, strings.HasSuffix(name, "Untrusted"),
				"%s.%s should not appear on the Gateway interface; *Untrusted variants are used internally by the gateway proxy",
				iface.Name(), name)
		}
	}

	t.Run("v1", func(t *testing.T) {
		check(t, reflect.TypeFor[api.Gateway]())
	})
	t.Run("v2", func(t *testing.T) {
		check(t, reflect.TypeFor[v2api.Gateway]())
	})
}

func TestGatewayRejectsOversizedEventRanges(t *testing.T) {
	ctx := context.Background()
	fromBlock := "0x64"
	toBlock := "0x1cd" // 461 - 100 = 361 epochs
	ethFilter := &ethtypes.EthFilterSpec{
		FromBlock: &fromBlock,
		ToBlock:   &toBlock,
	}
	fromHeight := abi.ChainEpoch(100)
	toHeight := abi.ChainEpoch(461)
	actorFilter := &types.ActorEventFilter{
		FromHeight: &fromHeight,
		ToHeight:   &toHeight,
	}

	cases := []struct {
		name string
		call func(*Node) error
	}{
		{"v1/EthGetLogs", func(gw *Node) error {
			_, err := gw.v1Proxy.EthGetLogs(ctx, ethFilter)
			return err
		}},
		{"v2/EthGetLogs", func(gw *Node) error {
			_, err := gw.v2Proxy.EthGetLogs(ctx, ethFilter)
			return err
		}},
		{"v1/EthNewFilter", func(gw *Node) error {
			_, err := gw.v1Proxy.EthNewFilter(ctx, ethFilter)
			return err
		}},
		{"v2/EthNewFilter", func(gw *Node) error {
			_, err := gw.v2Proxy.EthNewFilter(ctx, ethFilter)
			return err
		}},
		{"GetActorEventsRaw", func(gw *Node) error {
			_, err := gw.v1Proxy.GetActorEventsRaw(ctx, actorFilter)
			return err
		}},
		{"SubscribeActorEventsRaw", func(gw *Node) error {
			_, err := gw.v1Proxy.SubscribeActorEventsRaw(ctx, actorFilter)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockV1 := v1mocks.NewMockFullNode(ctrl)
			mockV2 := v2mocks.NewMockFullNode(ctrl)
			gw := NewNode(mockV1, mockV2)

			err := tc.call(gw)
			var rangeErr *api.ErrBlockRangeExceeded
			require.ErrorAs(t, err, &rangeErr)
			require.Equal(t, "block range exceeds maximum of 360 (got 361)", rangeErr.Error())
		})
	}
}

func TestGatewayEventFilterRangeBoundaries(t *testing.T) {
	ctx := context.Background()
	gw := &Node{eventFilterMaxHeightRange: DefaultEventFilterMaxHeightRange}

	require.NoError(t, gw.checkEventFilterHeightRange(100, 460))
	var rangeErr *api.ErrBlockRangeExceeded
	require.ErrorAs(t, gw.checkEventFilterHeightRange(100, 461), &rangeErr)

	fromBlock := "0x64"
	latest := ethtypes.BlockTagLatest
	ethFilter := &ethtypes.EthFilterSpec{
		FromBlock: &fromBlock,
		ToBlock:   &latest,
	}
	headHeight := func(context.Context) (abi.ChainEpoch, error) {
		// Event execution is available through the head's parent, height 460.
		return 461, nil
	}
	require.NoError(t, gw.checkEthEventFilterBlockRange(ctx, ethFilter, headHeight))
	headHeight = func(context.Context) (abi.ChainEpoch, error) {
		return 462, nil
	}
	rangeErr = nil
	require.ErrorAs(t, gw.checkEthEventFilterBlockRange(ctx, ethFilter, headHeight), &rangeErr)

	fromHeight := abi.ChainEpoch(100)
	actorFilter := &types.ActorEventFilter{FromHeight: &fromHeight}
	headHeight = func(context.Context) (abi.ChainEpoch, error) {
		return 461, nil
	}
	require.NoError(t, gw.checkActorEventFilterHeightRange(ctx, actorFilter, headHeight))
	headHeight = func(context.Context) (abi.ChainEpoch, error) {
		return 462, nil
	}
	rangeErr = nil
	require.ErrorAs(t, gw.checkActorEventFilterHeightRange(ctx, actorFilter, headHeight), &rangeErr)
}

func TestGatewayEventFilterRangeConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockV1 := v1mocks.NewMockFullNode(ctrl)
	mockV2 := v2mocks.NewMockFullNode(ctrl)

	gw := NewNode(mockV1, mockV2, WithEventFilterMaxHeightRange(10))
	require.NoError(t, gw.checkEventFilterHeightRange(100, 110))
	var rangeErr *api.ErrBlockRangeExceeded
	require.ErrorAs(t, gw.checkEventFilterHeightRange(100, 111), &rangeErr)

	gw = NewNode(mockV1, mockV2, WithEventFilterMaxHeightRange(0))
	require.NoError(t, gw.checkEventFilterHeightRange(0, 1_000_000))
}
