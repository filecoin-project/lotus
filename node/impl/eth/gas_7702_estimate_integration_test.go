package eth

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	abi2 "github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// mocks for estimation path
type mockEGTipsetResolver struct{ ts *types.TipSet }

func (m *mockEGTipsetResolver) GetTipSetByHash(ctx context.Context, h ethtypes.EthHash) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGTipsetResolver) GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGTipsetResolver) GetTipsetByBlockNumberOrHash(ctx context.Context, p ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	return m.ts, nil
}

type mockEGChainStore struct{ ts *types.TipSet }

func (m *mockEGChainStore) GetHeaviestTipSet() *types.TipSet { return m.ts }
func (m *mockEGChainStore) GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGChainStore) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGChainStore) GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGChainStore) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockEGChainStore) GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error) {
	return nil, nil
}
func (m *mockEGChainStore) GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error) {
	return nil, nil
}
func (m *mockEGChainStore) BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]store.BlockMessages, error) {
	return nil, nil
}
func (m *mockEGChainStore) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	return nil, nil
}
func (m *mockEGChainStore) ReadReceipts(ctx context.Context, root cid.Cid) ([]types.MessageReceipt, error) {
	return nil, nil
}
func (m *mockEGChainStore) ActorStore(ctx context.Context) adt.Store { return nil }

type mockEGStateManager struct{}

func (m *mockEGStateManager) GetNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version {
	return network.Version16
}
func (m *mockEGStateManager) TipSetState(ctx context.Context, ts *types.TipSet) (cid.Cid, cid.Cid, error) {
	return cid.Undef, cid.Undef, nil
}
func (m *mockEGStateManager) ParentState(ts *types.TipSet) (*state.StateTree, error) { return nil, nil }
func (m *mockEGStateManager) StateTree(st cid.Cid) (*state.StateTree, error)         { return nil, nil }
func (m *mockEGStateManager) LookupIDAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return addr, nil
}
func (m *mockEGStateManager) LoadActor(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	return nil, nil
}
func (m *mockEGStateManager) LoadActorRaw(ctx context.Context, addr address.Address, st cid.Cid) (*types.Actor, error) {
	return nil, nil
}
func (m *mockEGStateManager) ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return addr, nil
}
func (m *mockEGStateManager) ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	return cid.Undef, nil, nil
}
func (m *mockEGStateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockEGStateManager) CallOnState(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockEGStateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error) {
	rct := &types.MessageReceipt{ExitCode: 0, GasUsed: msg.GasLimit}
	return &api.InvocResult{MsgRct: rct}, nil
}
func (m *mockEGStateManager) ApplyOnStateWithGas(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockEGStateManager) HasExpensiveForkBetween(parent, height abi.ChainEpoch) bool {
	return false
}

type mockEGMessagePool struct {
	ts  *types.TipSet
	cfg types.MpoolConfig
}

func (m *mockEGMessagePool) PendingFor(ctx context.Context, a address.Address) ([]*types.SignedMessage, *types.TipSet) {
	return nil, m.ts
}
func (m *mockEGMessagePool) GetConfig() *types.MpoolConfig { return &m.cfg }

type mockEGGasAPI struct{ msg *types.Message }

func (m *mockEGGasAPI) GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, ts types.TipSetKey) (types.BigInt, error) {
	return types.NewInt(0), nil
}
func (m *mockEGGasAPI) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, ts types.TipSetKey) (*types.Message, error) {
	return m.msg, nil
}

func makeTipset2(t *testing.T) *types.TipSet {
	t.Helper()
	miner, _ := address.NewIDAddress(1000)
	// dummy cid
	var c cid.Cid
	b := []byte{0x01}
	c, _ = abi.CidBuilder.Sum(b)
	bh := &types.BlockHeader{Miner: miner, Height: 10, Ticket: &types.Ticket{VRFProof: []byte{1}}, ParentBaseFee: big.NewInt(1), ParentStateRoot: c, ParentMessageReceipts: c, Messages: c}
	ts, err := types.NewTipSet([]*types.BlockHeader{bh})
	require.NoError(t, err)
	return ts
}

func make7702ParamsN(t *testing.T, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	// Atomic: [ [ tuples... ], [to,value,input] ]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(n)))
	for i := 0; i < n; i++ {
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
		var a [20]byte
		a[0] = byte(i + 1)
		require.NoError(t, cbg.WriteByteArray(&buf, a[:]))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	}
	// call tuple [to(20), value, input]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	return buf.Bytes()
}

func TestEthEstimateGas_7702AddsSomeOverheadWhenTuplesPresent(t *testing.T) {
	ctx := context.Background()
	// Feature flag on and EthAccount.ApplyAndCall addr set
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	// fake gas API returns a message targeting ApplyAndCall with 2 tuples and base gaslimit 10000
	base := int64(10000)
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: make7702ParamsN(t, 2)}
	gas := &mockEGGasAPI{msg: msg}

	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	var rawTx ethtypes.EthCall
	b, _ := json.Marshal([]interface{}{rawTx})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	// Assert overhead increases the result beyond the base estimation, without pinning exact values.
	require.Greater(t, int64(got), base)
}

func TestEthEstimateGas_7702NoOverheadWhenDisabled(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = false
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	base := int64(8000)
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: make7702ParamsN(t, 1)}
	gas := &mockEGGasAPI{msg: msg}
	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(base), got)
}

func TestEthEstimateGas_NoOverheadForNonApplyAndCall(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	base := int64(7000)
	// Not targeting ApplyAndCall
	to, _ := address.NewIDAddress(1001)
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: to, Method: 0, GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1)}
	gas := &mockEGGasAPI{msg: msg}
	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(base), got)
}

func TestEthEstimateGas_ZeroTuple_NoOverhead(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	base := int64(6000)
	// Zero tuple wrapper params
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 0))
	// call tuple [to(20), value, input]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: buf.Bytes()}
	gas := &mockEGGasAPI{msg: msg}
	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(base), got)
}

func TestEthEstimateGas_7702OverheadScalesWithTupleCount(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	base := int64(9000)
	mk := func(n int) ethtypes.EthUint64 {
		msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: make7702ParamsN(t, n)}
		gas := &mockEGGasAPI{msg: msg}
		api := NewEthGasAPI(cs, sm, mp, gas, tr)
		b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
		p := jsonrpc.RawParams(b)
		got, err := api.EthEstimateGas(ctx, p)
		require.NoError(t, err)
		return got
	}

	g1 := mk(1)
	g2 := mk(2)
	require.Greater(t, int64(g2), int64(g1))
}

// legacy CBOR shape test removed: canonical wrapper-only + atomic is used

func TestEthEstimateGas_MalformedCBOR_NoOverhead(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	base := int64(7200)
	// Malformed CBOR: write an unsigned int instead of an array
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 7))
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: buf.Bytes()}
	gas := &mockEGGasAPI{msg: msg}
	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(base), got)
}

func TestEthEstimateGas_OverheadForApplyAndCall(t *testing.T) {
	ctx := context.Background()
	ethtypes.Eip7702FeatureEnabled = true
	defer func() { ethtypes.Eip7702FeatureEnabled = false }()

	// Configure an EthAccount.ApplyAndCall receiver
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999

	ts := makeTipset2(t)
	cs := &mockEGChainStore{ts: ts}
	sm := &mockEGStateManager{}
	tr := &mockEGTipsetResolver{ts: ts}
	mp := &mockEGMessagePool{ts: ts, cfg: types.MpoolConfig{GasLimitOverestimation: 1.0}}

	// Build atomic params with 2 tuples
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	writeTuple := func() {
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 314))
		require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
		require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	}
	writeTuple()
	writeTuple()
	// call tuple
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))

	base := int64(8000)
	msg := &types.Message{From: ts.Blocks()[0].Miner, To: ethtypes.EthAccountApplyAndCallActorAddr, Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: base, GasFeeCap: big.NewInt(1), GasPremium: big.NewInt(1), Params: buf.Bytes()}
	gas := &mockEGGasAPI{msg: msg}
	api := NewEthGasAPI(cs, sm, mp, gas, tr)
	b, _ := json.Marshal([]interface{}{ethtypes.EthCall{}})
	p := jsonrpc.RawParams(b)
	got, err := api.EthEstimateGas(ctx, p)
	require.NoError(t, err)
	// Expect overhead added for 2 tuples
	require.Greater(t, int64(got), base)
}
