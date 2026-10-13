package eth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestParseBlockRange(t *testing.T) {
	pstring := func(s string) *string { return &s }

	tcs := map[string]struct {
		heaviest abi.ChainEpoch
		from     *string
		to       *string
		maxRange abi.ChainEpoch
		minOut   abi.ChainEpoch
		maxOut   abi.ChainEpoch
		errStr   string
	}{
		"fails when both are specified and range is greater than max allowed range": {
			heaviest: 100,
			from:     pstring("0x100"),
			to:       pstring("0x200"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "block range exceeds maximum",
		},
		"fails when min is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     pstring("0x10"),
			to:       pstring("latest"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "block range exceeds maximum",
		},
		"fails when max is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     pstring("earliest"),
			to:       pstring("0x10000"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "block range exceeds maximum",
		},
		"works when range is valid": {
			heaviest: 500,
			from:     pstring("earliest"),
			to:       pstring("latest"),
			maxRange: 1000,
			minOut:   0,
			maxOut:   -1,
		},
		"works when range is valid and specified": {
			heaviest: 500,
			from:     pstring("0x10"),
			to:       pstring("0x30"),
			maxRange: 1000,
			minOut:   16,
			maxOut:   48,
		},
	}

	for name, tc := range tcs {
		tc2 := tc
		t.Run(name, func(t *testing.T) {
			min, max, err := parseBlockRange(tc2.heaviest, tc2.from, tc2.to, tc2.maxRange)
			require.Equal(t, tc2.minOut, min)
			require.Equal(t, tc2.maxOut, max)
			if tc2.errStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc2.errStr)
				var blockRangeErr *api.ErrBlockRangeExceeded
				require.ErrorAs(t, err, &blockRangeErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEthLogFromEvent(t *testing.T) {
	// basic empty
	data, topics, ok := ethLogFromEvent(nil)
	require.True(t, ok)
	require.Nil(t, data)
	require.Empty(t, topics)
	require.NotNil(t, topics)

	// basic topic
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.True(t, ok)
	require.Nil(t, data)
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})

	// basic topic with data
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: []byte{0x0},
	}})
	require.True(t, ok)
	require.Equal(t, data, []byte{0x0})
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})

	// skip topic
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t2",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// duplicate topic
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// duplicate data
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// unknown key is fine
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t5",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.True(t, ok)
	require.Nil(t, data)
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})
}

// ethSubscribeOnly exposes EthSubscribe alone over JSON-RPC so the test can
// reach it through a websocket connection, which is what supplies the reverse
// client EthSubscribe needs.
type ethSubscribeOnly struct {
	events *ethEvents
}

func (s *ethSubscribeOnly) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	return s.events.EthSubscribe(ctx, p)
}

func TestEthSubscribeRejectedParamsStartNoSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subMgr := NewEthSubscriptionManager(nil, nil, nil)
	events := &ethEvents{subscriptionCtx: ctx, subscriptionManager: subMgr}

	rpcServer := jsonrpc.NewServer(jsonrpc.WithReverseClient[api.EthSubscriberMethods]("Filecoin"))
	rpcServer.Register("Filecoin", &ethSubscribeOnly{events: events})
	srv := httptest.NewServer(rpcServer)
	defer srv.Close()

	var client struct {
		EthSubscribe func(context.Context, jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error)
	}
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+srv.Listener.Addr().String(), "Filecoin", []any{&client}, nil)
	require.NoError(t, err)
	defer closer()

	activeSubs := func() int {
		subMgr.mu.Lock()
		defer subMgr.mu.Unlock()
		return len(subMgr.subs)
	}

	for _, params := range []string{
		`["bogus"]`,
		// a masked ID address whose ID is 2^63, which ToFilecoinAddress rejects
		`["logs",{"address":"0xff00000000000000000000008000000000000000"}]`,
	} {
		_, err := client.EthSubscribe(ctx, jsonrpc.RawParams(params))
		require.Error(t, err)
		require.Zero(t, activeSubs(), "rejected eth_subscribe %s started a subscription", params)
	}
}

func TestEthFilterLogsFromEvents(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemorySync()
	cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), nil, nil)
	t.Cleanup(func() { require.NoError(t, cs.Close()) })
	addr, err := (ethtypes.EthAddress{1}).ToFilecoinAddress()
	require.NoError(t, err)
	msg := &types.Message{
		From: addr, To: addr,
		Value: big.Zero(), GasFeeCap: big.Zero(), GasPremium: big.Zero(),
	}
	readable, err := cs.PutMessage(ctx, msg)
	require.NoError(t, err)
	msg.Nonce++
	missing := msg.Cid()
	missingErr := format.ErrNotFound{Cid: missing}
	wantHash, err := ethtypes.EthHashFromCid(readable)
	require.NoError(t, err)

	// Exercise the real unsigned/BLS fallback after signed decoding fails.
	_, err = cs.GetSignedMessage(ctx, readable)
	require.Error(t, err)
	event := func(c cid.Cid) *index.CollectedEvent {
		return &index.CollectedEvent{
			MsgCid: c, EmitterAddr: addr, TipSetKey: types.NewTipSetKey(readable),
			Entries: []types.EventEntry{{Key: "d", Codec: cid.Raw, Value: []byte{1}}},
		}
	}
	nonEth := event(missing)
	nonEth.Entries[0].Codec = cid.DagCBOR

	for _, tc := range []struct {
		name     string
		events   []*index.CollectedEvent
		wantLogs int
		wantErr  error
	}{
		{"BLS fallback", []*index.CollectedEvent{event(readable)}, 1, nil},
		{"missing message", []*index.CollectedEvent{event(missing)}, 0, missingErr},
		{"readable then missing", []*index.CollectedEvent{event(readable), event(missing)}, 0, missingErr},
		{"empty", nil, 0, nil},
		{"non-Ethereum", []*index.CollectedEvent{nonEth}, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, err := ethFilterLogsFromEvents(ctx, tc.events, cs, nil)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorContains(t, err, missing.String())
				require.Nil(t, logs, "must not return partial logs")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, logs)
			require.Len(t, logs, tc.wantLogs)
			if tc.wantLogs > 0 {
				require.Equal(t, wantHash, logs[0].TransactionHash)
			}
		})
	}
}

type eventMessageReadErrors struct {
	ChainStore
	signedErr   error
	unsignedErr error
}

func (s eventMessageReadErrors) GetSignedMessage(context.Context, cid.Cid) (*types.SignedMessage, error) {
	return nil, s.signedErr
}

func (s eventMessageReadErrors) GetMessage(context.Context, cid.Cid) (*types.Message, error) {
	return nil, s.unsignedErr
}

func TestEthTxHashFromMessageCidReadErrors(t *testing.T) {
	c, err := abi.CidBuilder.Sum([]byte("event message"))
	require.NoError(t, err)
	for _, cause := range []error{errors.New("storage failure"), context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			// The fallback must not mask the original read failure.
			unsignedErr := errors.New("unsigned message read failed")
			cs := eventMessageReadErrors{signedErr: cause, unsignedErr: unsignedErr}
			_, err := ethTxHashFromMessageCid(context.Background(), c, cs)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, unsignedErr)
			require.ErrorContains(t, err, c.String())
		})
	}
}
