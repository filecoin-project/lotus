// stm: #unit
package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestGatewayAPIChainGetTipSetByHeight(t *testing.T) {
	ctx := context.Background()

	lookbackTimestamp := uint64(time.Now().Unix()) - uint64(DefaultLookbackCap.Seconds())
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
			mock := &mockGatewayDepsAPI{}
			a := NewNode(mock, nil, DefaultLookbackCap, DefaultStateWaitLookbackLimit, 0, time.Minute)

			// Create tipsets from genesis up to tskh and return the highest
			ts := mock.createTipSets(tt.args.tskh, tt.args.genesisTS)

			//stm: @GATEWAY_NODE_GET_TIPSET_BY_HEIGHT_001
			got, err := a.ChainGetTipSetByHeight(ctx, tt.args.h, ts.Key())
			if tt.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.args.h, got.Height())
			}
		})
	}
}

type mockGatewayDepsAPI struct {
	lk      sync.RWMutex
	tipsets []*types.TipSet

	TargetAPI // satisfies all interface requirements but will panic if
	// methods are called. easier than filling out with panic stubs IMO
}

func (m *mockGatewayDepsAPI) ChainHasObj(context.Context, cid.Cid) (bool, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) ChainReadObj(ctx context.Context, c cid.Cid) ([]byte, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	return m.tipsets[len(m.tipsets)-1], nil
}

func (m *mockGatewayDepsAPI) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	for _, ts := range m.tipsets {
		if ts.Key() == tsk {
			return ts, nil
		}
	}

	return nil, nil
}

// createTipSets creates tipsets from genesis up to tskh and returns the highest
func (m *mockGatewayDepsAPI) createTipSets(h abi.ChainEpoch, genesisTimestamp uint64) *types.TipSet {
	m.lk.Lock()
	defer m.lk.Unlock()

	targeth := h + 1 // add one for genesis block
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
		m.tipsets = append(m.tipsets, currts)
	}

	return m.tipsets[len(m.tipsets)-1]
}

func (m *mockGatewayDepsAPI) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.tipsets[h], nil
}

func (m *mockGatewayDepsAPI) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MpoolPushUntrusted(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateWaitMsgLimited(ctx context.Context, msg cid.Cid, confidence uint64, h abi.ChainEpoch) (*api.MsgLookup, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) StateReadState(ctx context.Context, act address.Address, ts types.TipSetKey) (*api.ActorState, error) {
	panic("implement me")
}

func (m *mockGatewayDepsAPI) Version(context.Context) (api.APIVersion, error) {
	return api.APIVersion{
		APIVersion: api.FullAPIVersion1,
	}, nil
}

func TestGatewayVersion(t *testing.T) {
	//stm: @GATEWAY_NODE_GET_VERSION_001
	ctx := context.Background()
	mock := &mockGatewayDepsAPI{}
	a := NewNode(mock, nil, DefaultLookbackCap, DefaultStateWaitLookbackLimit, 0, time.Minute)

	v, err := a.Version(ctx)
	require.NoError(t, err)
	require.Equal(t, api.FullAPIVersion1, v.APIVersion)
}

func TestGatewayLimitTokensAvailable(t *testing.T) {
	ctx := context.Background()
	mock := &mockGatewayDepsAPI{}
	tokens := 3
	a := NewNode(mock, nil, DefaultLookbackCap, DefaultStateWaitLookbackLimit, int64(tokens), time.Minute)
	require.NoError(t, a.limit(ctx, tokens), "requests should not be limited when there are enough tokens available")
}

func TestGatewayLimitTokensRate(t *testing.T) {
	ctx := context.Background()
	mock := &mockGatewayDepsAPI{}
	tokens := 3
	var rateLimit int64 = 200
	rateLimitTimeout := time.Second / time.Duration(rateLimit/3) // large enough to not be hit
	a := NewNode(mock, nil, DefaultLookbackCap, DefaultStateWaitLookbackLimit, rateLimit, rateLimitTimeout)

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
	a = NewNode(mock, nil, DefaultLookbackCap, DefaultStateWaitLookbackLimit, rateLimit, rateLimitTimeout)
	require.NoError(t, a.limit(ctx, tokens))
	require.ErrorContains(t, a.limit(ctx, tokens), "server busy", "API calls should be hard rate limited when they hit limits")
}
