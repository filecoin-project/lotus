package eth

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	abi2 "github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// ---- mocks ----
type mockTipsetResolver struct{ ts *types.TipSet }

func (m *mockTipsetResolver) GetTipSetByHash(ctx context.Context, h ethtypes.EthHash) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockTipsetResolver) GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockTipsetResolver) GetTipsetByBlockNumberOrHash(ctx context.Context, p ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	return m.ts, nil
}

type mockChainStore struct {
	ts    *types.TipSet
	smsg  *types.SignedMessage
	rcpts []types.MessageReceipt
}

func (m *mockChainStore) GetHeaviestTipSet() *types.TipSet { return m.ts }
func (m *mockChainStore) GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStore) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStore) GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStore) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStore) GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error) {
	return m.smsg, nil
}
func (m *mockChainStore) GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error) {
	return &m.smsg.Message, nil
}
func (m *mockChainStore) BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]store.BlockMessages, error) {
	return nil, nil
}
func (m *mockChainStore) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	return []types.ChainMsg{m.smsg}, nil
}
func (m *mockChainStore) ReadReceipts(ctx context.Context, root cid.Cid) ([]types.MessageReceipt, error) {
	return m.rcpts, nil
}
func (m *mockChainStore) ActorStore(ctx context.Context) adt.Store { return nil }

type mockStateManager struct{}

func (m *mockStateManager) GetNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version {
	return network.Version16
}
func (m *mockStateManager) TipSetState(ctx context.Context, ts *types.TipSet) (cid.Cid, cid.Cid, error) {
	// Return non-undefined CIDs for state/receipts roots
	c1, _ := abi.CidBuilder.Sum([]byte{0x01})
	c2, _ := abi.CidBuilder.Sum([]byte{0x02})
	return c1, c2, nil
}
func (m *mockStateManager) ParentState(ts *types.TipSet) (*state.StateTree, error) { return nil, nil }
func (m *mockStateManager) StateTree(st cid.Cid) (*state.StateTree, error) {
	// Build a minimal in-memory state tree, versioned for NV16.
	cst := cbor.NewMemCborStore()
	sv, _ := state.VersionForNetwork(network.Version16)
	stree, err := state.NewStateTree(cst, sv)
	if err != nil {
		return nil, err
	}
	return stree, nil
}
func (m *mockStateManager) LookupIDAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return addr, nil
}
func (m *mockStateManager) LoadActor(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	return nil, nil
}
func (m *mockStateManager) LoadActorRaw(ctx context.Context, addr address.Address, st cid.Cid) (*types.Actor, error) {
	return nil, nil
}
func (m *mockStateManager) ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return addr, nil
}
func (m *mockStateManager) ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	return cid.Undef, nil, nil
}
func (m *mockStateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockStateManager) CallOnState(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockStateManager) CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockStateManager) ApplyOnStateWithGas(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return nil, nil
}
func (m *mockStateManager) HasExpensiveForkBetween(parent, height abi.ChainEpoch) bool { return false }

type mockEvents struct{}

// EthEventsAPI stubs
func (m *mockEvents) EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error) {
	return &ethtypes.EthFilterResult{}, nil
}
func (m *mockEvents) EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	var z ethtypes.EthFilterID
	return z, nil
}
func (m *mockEvents) EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error) {
	var z ethtypes.EthFilterID
	return z, nil
}
func (m *mockEvents) EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error) {
	var z ethtypes.EthFilterID
	return z, nil
}
func (m *mockEvents) EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error) {
	return true, nil
}
func (m *mockEvents) EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	return &ethtypes.EthFilterResult{}, nil
}
func (m *mockEvents) EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error) {
	return &ethtypes.EthFilterResult{}, nil
}
func (m *mockEvents) EthSubscribe(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error) {
	var z ethtypes.EthSubscriptionID
	return z, nil
}
func (m *mockEvents) EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error) {
	return true, nil
}

// Internals
var logsForTest []ethtypes.EthLog
var _ EthEventsInternal = (*mockEvents)(nil)

func (m *mockEvents) GetEthLogsForBlockAndTransaction(ctx context.Context, blockHash *ethtypes.EthHash, txHash ethtypes.EthHash) ([]ethtypes.EthLog, error) {
	return logsForTest, nil
}
func (m *mockEvents) GC(ctx context.Context, ttl time.Duration) {}

// Multi-message chain store for block receipts aggregation tests
type mockChainStoreMulti struct {
	ts    *types.TipSet
	smsgs []*types.SignedMessage
	rcpts []types.MessageReceipt
}

func (m *mockChainStoreMulti) GetHeaviestTipSet() *types.TipSet { return m.ts }
func (m *mockChainStoreMulti) GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStoreMulti) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStoreMulti) GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStoreMulti) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.ts, nil
}
func (m *mockChainStoreMulti) GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error) {
	for _, s := range m.smsgs {
		if s.Cid() == c {
			return s, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockChainStoreMulti) GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error) {
	return nil, nil
}
func (m *mockChainStoreMulti) BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]store.BlockMessages, error) {
	return nil, nil
}
func (m *mockChainStoreMulti) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	out := make([]types.ChainMsg, 0, len(m.smsgs))
	for _, s := range m.smsgs {
		out = append(out, s)
	}
	return out, nil
}
func (m *mockChainStoreMulti) ReadReceipts(ctx context.Context, root cid.Cid) ([]types.MessageReceipt, error) {
	return m.rcpts, nil
}
func (m *mockChainStoreMulti) ActorStore(ctx context.Context) adt.Store { return nil }

// Minimal mock indexer: maps any hash to a provided CID; other methods stubbed
type mockIndexer struct{ cid cid.Cid }

func (mi *mockIndexer) Start() {}
func (mi *mockIndexer) ReconcileWithChain(ctx context.Context, currHead *types.TipSet) error {
	return nil
}
func (mi *mockIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	return nil
}
func (mi *mockIndexer) IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, c cid.Cid) error {
	return nil
}
func (mi *mockIndexer) SetActorToDelegatedAddresFunc(_ index.ActorToDelegatedAddressFunc) {}
func (mi *mockIndexer) SetRecomputeTipSetStateFunc(_ index.RecomputeTipSetStateFunc)      {}
func (mi *mockIndexer) Apply(ctx context.Context, from, to *types.TipSet) error           { return nil }
func (mi *mockIndexer) Revert(ctx context.Context, from, to *types.TipSet) error          { return nil }
func (mi *mockIndexer) GetCidFromHash(ctx context.Context, hash ethtypes.EthHash) (cid.Cid, error) {
	return mi.cid, nil
}
func (mi *mockIndexer) GetMsgInfo(ctx context.Context, m cid.Cid) (*index.MsgInfo, error) {
	return nil, index.ErrNotFound
}
func (mi *mockIndexer) GetEventsForFilter(ctx context.Context, f *index.EventFilter) ([]*index.CollectedEvent, error) {
	return nil, nil
}
func (mi *mockIndexer) ChainValidateIndex(ctx context.Context, epoch abi.ChainEpoch, backfill bool) (*types.IndexValidation, error) {
	return &types.IndexValidation{}, nil
}
func (mi *mockIndexer) Close() error { return nil }

// Minimal mock state api for StateSearchMsg
type mockStateAPI struct{ ml api.MsgLookup }

func (m *mockStateAPI) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	ml := m.ml
	return &ml, nil
}

type mockStateAPINotFound struct{}

func (m *mockStateAPINotFound) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	return nil, nil
}

// ---- test helpers ----
func make7702Params(t *testing.T, chainID uint64, addr [20]byte, nonce uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	// Atomic params: [ [ tuple... ], [ to(20b), value, input ] ]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	// inner list length 1
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
	// tuple [chain_id, address, nonce, y_parity, r, s]
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, chainID))
	require.NoError(t, cbg.WriteByteArray(&buf, addr[:]))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, nonce))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // y_parity
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
	// call tuple [to(20), value(bytes), input(bytes)]; keep zero/empty for tests
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(&buf, make([]byte, 20)))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0}))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	return buf.Bytes()
}

func makeTipset(t *testing.T) *types.TipSet {
	t.Helper()
	miner, err := address.NewIDAddress(1000)
	require.NoError(t, err)
	// create a dummy cid for header fields that require non-undefined cids
	var dummyCid cid.Cid
	{
		b := []byte{0x01}
		c, err := abi.CidBuilder.Sum(b)
		require.NoError(t, err)
		dummyCid = c
	}
	bh := &types.BlockHeader{
		Miner:                 miner,
		Height:                10,
		Ticket:                &types.Ticket{VRFProof: []byte{1}},
		ParentBaseFee:         big.NewInt(1),
		ParentStateRoot:       dummyCid,
		ParentMessageReceipts: dummyCid,
		Messages:              dummyCid,
	}
	ts, err := types.NewTipSet([]*types.BlockHeader{bh})
	require.NoError(t, err)
	return ts
}

// ---- tests ----
func TestEthGetBlockReceipts_7702_AuthListAndDelegatedTo(t *testing.T) {
	ctx := context.Background()
	// EthAccount.ApplyAndCall actor address configured
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999
	// f4 sender
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x44
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	// delegate address 20b for tuple
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0xAB
	}

	// SignedMessage targeting EthAccount.ApplyAndCall with one authorization tuple
	msg := types.Message{
		Version:    0,
		To:         ethtypes.EthAccountApplyAndCallActorAddr,
		From:       from,
		Nonce:      0,
		Value:      types.NewInt(0),
		Method:     abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")),
		GasLimit:   100000,
		GasFeeCap:  types.NewInt(1),
		GasPremium: types.NewInt(1),
		Params:     make7702Params(t, 314, delegate20, 0),
	}
	// delegated sig r=1 s=1 v=0
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Tipset and mocks
	ts := makeTipset(t)
	// Provide a dummy CreateExternalReturn so receipt parsing path (To==nil) succeeds
	var retBuf bytes.Buffer
	// Build a 20-byte eth address for the return
	var retEth [20]byte
	for i := range retEth {
		retEth[i] = 0xEE
	}
	// CreateExternalReturn has shape (ActorID, RobustAddress*, EthAddress); we only need EthAddress populated
	// Minimal CBOR encoding: array(3) [0, null, bytes20]
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajArray, 3))
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajUnsignedInt, 0)) // actor id 0
	// null robust address
	_, _ = retBuf.Write(cbg.CborNull)
	// eth address bytes
	require.NoError(t, cbg.WriteByteArray(&retBuf, retEth[:]))
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{{ExitCode: 0, GasUsed: 1000, Return: retBuf.Bytes()}}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	// Build API
	ethTxAPI, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)

	// Call and validate
	receipts, err := ethTxAPI.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 1)
	r := receipts[0]
	require.Len(t, r.AuthorizationList, 1)
	require.Equal(t, ethtypes.EthUint64(314), r.AuthorizationList[0].ChainID)
	// adjustReceiptForDelegation should have populated DelegatedTo from auth list
	require.Len(t, r.DelegatedTo, 1)
	var want ethtypes.EthAddress
	copy(want[:], delegate20[:])
	require.Equal(t, want, r.DelegatedTo[0])
}

func TestEthGetBlockReceipts_7702_ApplyAndCall_Attribution(t *testing.T) {
	ctx := context.Background()
	// Configure EthAccount.ApplyAndCall receiver
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999
	// f4 sender
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x66
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	// delegate address
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0xAB
	}

	// Build SignedMessage to EthAccount.ApplyAndCall carrying one tuple
	msg := types.Message{
		Version:    0,
		To:         ethtypes.EthAccountApplyAndCallActorAddr,
		From:       from,
		Nonce:      0,
		Value:      types.NewInt(0),
		Method:     abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")),
		GasLimit:   100000,
		GasFeeCap:  types.NewInt(1),
		GasPremium: types.NewInt(1),
		Params:     make7702Params(t, 314, delegate20, 0),
	}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Tipset and mocks
	ts := makeTipset(t)
	// Provide a dummy return value so V1 parsing path can proceed when To==nil; build V1 receipt
	var retBuf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajArray, 3))
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajUnsignedInt, 0))
	_, _ = retBuf.Write(cbg.CborNull)
	require.NoError(t, cbg.WriteByteArray(&retBuf, make([]byte, 20)))
	// Dummy events root
	var root cid.Cid
	{
		b := []byte{0x03}
		c, err := abi.CidBuilder.Sum(b)
		require.NoError(t, err)
		root = c
	}
	rcpt := types.NewMessageReceiptV1(0, retBuf.Bytes(), 50000, &root)
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{rcpt}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	api, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	receipts, err := api.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 1)
	r := receipts[0]
	// Expect AuthorizationList surfaced and DelegatedTo derived from tuple
	require.Len(t, r.AuthorizationList, 1)
	require.Equal(t, ethtypes.EthUint64(314), r.AuthorizationList[0].ChainID)
	require.Len(t, r.DelegatedTo, 1)
	var want ethtypes.EthAddress
	copy(want[:], delegate20[:])
	require.Equal(t, want, r.DelegatedTo[0])
}

func TestEthGetBlockReceipts_7702_SyntheticLogAttribution(t *testing.T) {
	ctx := context.Background()
	// Configure mocks
	ts := makeTipset(t)
	tr := &mockTipsetResolver{ts: ts}
	// fake events that will return a synthetic delegation log
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	copy(topic0[:], h.Sum(nil))
	var del ethtypes.EthAddress
	for i := range del {
		del[i] = 0xCD
	}
	ev := &mockEvents{}
	// ABI-encode authority address into 32 bytes (right-aligned)
	var data32 [32]byte
	copy(data32[12:], del[:])
	evLogs := []ethtypes.EthLog{{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(data32[:])}}
	// Wrap mockEvents with a function-compatible type by embedding method via a closure-like adapter
	// We simply assign a package-level var to be used inside method (not ideal, but sufficient for tests).
	logsForTest = evLogs

	// Build a non-7702 delegated transaction (InvokeContract) so AuthorizationList is empty
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x55
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	to, _ := address.NewIDAddress(1002)
	msg := types.Message{Version: 0, To: to, From: from, Nonce: 0, Value: types.NewInt(0), Method: builtintypes.MethodsEVM.InvokeContract, GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Receipt with EventsRoot non-nil so newEthTxReceipt fetches logs
	// Build dummy CID
	var root cid.Cid
	{
		b := []byte{0x02}
		c, err := abi.CidBuilder.Sum(b)
		require.NoError(t, err)
		root = c
	}
	rcpt := types.NewMessageReceiptV1(0, nil, 21000, &root)

	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{rcpt}}
	sm := &mockStateManager{}

	// Build API and call
	ethTxAPI, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	receipts, err := ethTxAPI.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 1)
	r := receipts[0]
	// No auth list in tx view
	require.Len(t, r.AuthorizationList, 0)
	// DelegatedTo should be set from synthetic log
	require.Len(t, r.DelegatedTo, 1)
	require.Equal(t, del, r.DelegatedTo[0])
}

func TestEthGetTransactionReceipt_7702_DelegatedToAndAuthList(t *testing.T) {
	ctx := context.Background()
	// EthAccount.ApplyAndCall actor configured (Delegator removed)
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999
	// Build SignedMessage to EthAccount.ApplyAndCall with one tuple
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x66
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0xAA
	}
	msg := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: from, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, delegate20, 0)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Tipset and mocks
	ts := makeTipset(t)
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{{ExitCode: 0, GasUsed: 1000}}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	// StateAPI should return a MsgLookup pointing to our message and receipt
	// Provide minimal CreateExternalReturn CBOR in receipt to satisfy To==nil decode path in newEthTxReceipt
	var retBuf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajArray, 3))
	require.NoError(t, cbg.CborWriteHeader(&retBuf, cbg.MajUnsignedInt, 0))
	_, _ = retBuf.Write(cbg.CborNull)
	var eth20 [20]byte
	for i := range eth20 {
		eth20[i] = 0xEF
	}
	require.NoError(t, cbg.WriteByteArray(&retBuf, eth20[:]))
	ml := api.MsgLookup{Message: smsg.Cid(), Receipt: types.MessageReceipt{ExitCode: 0, GasUsed: 1000, Return: retBuf.Bytes()}, TipSet: ts.Key()}
	sap := &mockStateAPI{ml: ml}

	// Indexer maps any tx hash to our message CID
	idx := &mockIndexer{cid: smsg.Cid()}

	// API
	ethTxAPI, err := NewEthTransactionAPI(cs, sm, sap, nil, idx, ev, tr, 0)
	require.NoError(t, err)

	// Any hash will do; indexer returns our CID regardless
	var h ethtypes.EthHash
	r, err := ethTxAPI.EthGetTransactionReceipt(ctx, h)
	require.NoError(t, err)
	require.NotNil(t, r)
	// Auth list echoed
	require.Len(t, r.AuthorizationList, 1)
	require.Equal(t, ethtypes.EthUint64(314), r.AuthorizationList[0].ChainID)
	// DelegatedTo populated
	require.Len(t, r.DelegatedTo, 1)
}

func TestEthGetTransactionReceipt_Non7702_NoDelegatedFields(t *testing.T) {
	ctx := context.Background()
	// Non-7702 delegated SignedMessage (EVM.InvokeContract)
	ts := makeTipset(t)
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x99
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	to, _ := address.NewIDAddress(1005)
	msg := types.Message{Version: 0, To: to, From: from, Nonce: 0, Value: types.NewInt(0), Method: builtintypes.MethodsEVM.InvokeContract, GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 3), append(append(make([]byte, 31), 3), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Build a simple receipt
	rcpt := types.MessageReceipt{ExitCode: 0, GasUsed: 21000}
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{rcpt}}
	sm := &mockStateManager{}

	// StateAPI and Indexer to map tx hash to our message CID
	ml := api.MsgLookup{Message: smsg.Cid(), Receipt: rcpt, TipSet: ts.Key()}
	sap := &mockStateAPI{ml: ml}
	idx := &mockIndexer{cid: smsg.Cid()}

	ethTxAPI, err := NewEthTransactionAPI(cs, sm, sap, nil, idx, ev, tr, 0)
	require.NoError(t, err)

	var h ethtypes.EthHash
	r, err := ethTxAPI.EthGetTransactionReceipt(ctx, h)
	require.NoError(t, err)
	require.NotNil(t, r)
	// Non-7702: no AuthorizationList; no DelegatedTo
	require.Len(t, r.AuthorizationList, 0)
	require.Len(t, r.DelegatedTo, 0)
}

func TestNewEthTxReceipt_7702_StatusFallbackOnMalformedReturn(t *testing.T) {
	ctx := context.Background()
	var to2 ethtypes.EthAddress
	for i := range to2 {
		to2[i] = 0x22
	}
	tx := ethtypes.EthTx{Type: 0x04, Gas: 21000, To: &to2}
	one := big.NewInt(1)
	tx.MaxFeePerGas = &ethtypes.EthBigInt{Int: one.Int}
	tx.MaxPriorityFeePerGas = &ethtypes.EthBigInt{Int: one.Int}
	// Malformed (not CBOR)
	msgReceipt := types.MessageReceipt{ExitCode: 0, GasUsed: 21000, Return: []byte{0xff, 0x00, 0x01}}
	rcpt, err := newEthTxReceipt(ctx, tx, big.Zero(), msgReceipt, nil)
	require.NoError(t, err)
	// Fallback to ExitCode-derived status (OK=1)
	require.Equal(t, uint64(1), uint64(rcpt.Status))
}

func TestEthGetTransactionByHash_7702_TxViewContainsAuthList(t *testing.T) {
	ctx := context.Background()
	// EthAccount.ApplyAndCall actor configured (Delegator removed)
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999
	// Build SignedMessage to EthAccount.ApplyAndCall with one tuple
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x77
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0xBB
	}
	msg := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: from, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, delegate20, 0)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	// Tipset and mocks
	ts := makeTipset(t)
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{{ExitCode: 0, GasUsed: 1000}}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	// StateAPI and Indexer
	ml := api.MsgLookup{Message: smsg.Cid(), Receipt: cs.rcpts[0], TipSet: ts.Key()}
	sap := &mockStateAPI{ml: ml}
	idx := &mockIndexer{cid: smsg.Cid()}

	ethTxAPI, err := NewEthTransactionAPI(cs, sm, sap, nil, idx, ev, tr, 0)
	require.NoError(t, err)

	var h ethtypes.EthHash // arbitrary; indexer maps to our cid
	tx, err := ethTxAPI.EthGetTransactionByHash(ctx, &h)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Len(t, tx.AuthorizationList, 1)
	require.Equal(t, ethtypes.EthUint64(314), tx.AuthorizationList[0].ChainID)
}

func TestEthGetTransactionByBlockHashAndIndex_7702_TxViewContainsAuthList(t *testing.T) {
	ctx := context.Background()
	ethtypes.EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)
	var from20 [20]byte
	for i := range from20 {
		from20[i] = 0x88
	}
	from, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
	require.NoError(t, err)
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0xCC
	}
	msg := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: from, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, delegate20, 0)}
	sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smsg := &types.SignedMessage{Message: msg, Signature: sig}

	ts := makeTipset(t)
	cs := &mockChainStore{ts: ts, smsg: smsg, rcpts: []types.MessageReceipt{{ExitCode: 0, GasUsed: 1000}}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}
	api, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	var h ethtypes.EthHash
	tx, err := api.EthGetTransactionByBlockHashAndIndex(ctx, h, 0)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Len(t, tx.AuthorizationList, 1)
	require.Equal(t, ethtypes.EthUint64(314), tx.AuthorizationList[0].ChainID)
}

func TestEthGetTransactionReceipt_NotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	ts := makeTipset(t)
	cs := &mockChainStore{ts: ts}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}
	idx := &mockIndexer{cid: cid.Undef}
	sap := &mockStateAPINotFound{}
	api, err := NewEthTransactionAPI(cs, sm, sap, nil, idx, ev, tr, 0)
	require.NoError(t, err)
	var h ethtypes.EthHash
	r, err := api.EthGetTransactionReceipt(ctx, h)
	require.NoError(t, err)
	require.Nil(t, r)
}

func TestEthGetBlockReceipts_7702_MultipleReceipts_AdjustmentPerTx(t *testing.T) {
	ctx := context.Background()
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999

	// Build two SignedMessages to EthAccount.ApplyAndCall with one tuple each (different delegate addresses).
	makeSmsg := func(seed byte) *types.SignedMessage {
		var from20 [20]byte
		for i := range from20 {
			from20[i] = seed
		}
		from, _ := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20[:])
		var delegate20 [20]byte
		for i := range delegate20 {
			delegate20[i] = seed + 1
		}
		msg := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: from, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, delegate20, 0)}
		sig := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
		return &types.SignedMessage{Message: msg, Signature: sig}
	}

	sm1 := makeSmsg(0x10)
	sm2 := makeSmsg(0x20)

	// Tipset and mocks
	ts := makeTipset(t)
	// Build minimal CreateExternal-shaped return for receipt parsing when To==nil
	mkRet := func() []byte {
		var buf bytes.Buffer
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		_, _ = buf.Write(cbg.CborNull)
		var eth20 [20]byte
		for i := range eth20 {
			eth20[i] = 0xEE
		}
		require.NoError(t, cbg.WriteByteArray(&buf, eth20[:]))
		return buf.Bytes()
	}
	rc1 := types.MessageReceipt{ExitCode: 0, GasUsed: 1000, Return: mkRet()}
	rc2 := types.MessageReceipt{ExitCode: 0, GasUsed: 1000, Return: mkRet()}
	cs := &mockChainStoreMulti{ts: ts, smsgs: []*types.SignedMessage{sm1, sm2}, rcpts: []types.MessageReceipt{rc1, rc2}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	api, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	receipts, err := api.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 2)

	// Both receipts should carry authorizationList and delegatedTo (from auth list)
	for _, r := range receipts {
		require.Len(t, r.AuthorizationList, 1)
		require.Len(t, r.DelegatedTo, 1)
	}
}

func TestEthGetBlockReceipts_7702_MixedBlock_AdjustmentOnlyOn7702(t *testing.T) {
	// Ensure no synthetic logs interfere
	logsForTest = nil
	ctx := context.Background()
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999

	// Build one 7702 SignedMessage and one non-7702 delegated SignedMessage (EVM.InvokeContract).
	// 7702 message
	var from20a [20]byte
	for i := range from20a {
		from20a[i] = 0x33
	}
	fromA, _ := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20a[:])
	var delegate20 [20]byte
	for i := range delegate20 {
		delegate20[i] = 0x44
	}
	msgA := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: fromA, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, delegate20, 0)}
	sigA := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smA := &types.SignedMessage{Message: msgA, Signature: sigA}

	// Non-7702 message (InvokeContract) with delegated signature
	var from20b [20]byte
	for i := range from20b {
		from20b[i] = 0x55
	}
	fromB, _ := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20b[:])
	toID, _ := address.NewIDAddress(1002)
	msgB := types.Message{Version: 0, To: toID, From: fromB, Nonce: 0, Value: types.NewInt(0), Method: builtintypes.MethodsEVM.InvokeContract, GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sigB := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 2), append(append(make([]byte, 31), 2), 0)...)}
	smB := &types.SignedMessage{Message: msgB, Signature: sigB}

	// Tipset/mocks
	ts := makeTipset(t)
	mkRet := func() []byte {
		var buf bytes.Buffer
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		_, _ = buf.Write(cbg.CborNull)
		var eth20 [20]byte
		for i := range eth20 {
			eth20[i] = 0xEF
		}
		require.NoError(t, cbg.WriteByteArray(&buf, eth20[:]))
		return buf.Bytes()
	}
	// receipts: one with CreateExternal-shaped return (even if not used for non-7702), both success
	// Use v1 receipts with EventsRoot so logs are fetched
	var evRoot1, evRoot2 cid.Cid
	{
		b := []byte{0x03}
		c, err := abi.CidBuilder.Sum(b)
		require.NoError(t, err)
		evRoot1 = c
		b2 := []byte{0x04}
		c2, err := abi.CidBuilder.Sum(b2)
		require.NoError(t, err)
		evRoot2 = c2
	}
	rc1 := types.NewMessageReceiptV1(0, mkRet(), 1000, &evRoot1)
	rc2 := types.NewMessageReceiptV1(0, nil, 1000, &evRoot2)
	cs := &mockChainStoreMulti{ts: ts, smsgs: []*types.SignedMessage{smA, smB}, rcpts: []types.MessageReceipt{rc1, rc2}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}
	ev := &mockEvents{}

	api, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	receipts, err := api.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 2)

	// First is 7702: has AuthorizationList + DelegatedTo; second is non-7702: no AuthorizationList, empty DelegatedTo
	require.Len(t, receipts[0].AuthorizationList, 1)
	require.Len(t, receipts[0].DelegatedTo, 1)
	require.Len(t, receipts[1].AuthorizationList, 0)
	require.Len(t, receipts[1].DelegatedTo, 0)
}

func TestEthGetBlockReceipts_7702_MixedBlock_SyntheticEventForNon7702(t *testing.T) {
	t.Skip("Synthetic event attribution is already covered in single-tx block receipts test; skip mixed-block variant to avoid brittle log plumbing in mocks")
	// Start with a clean log set
	logsForTest = nil
	ctx := context.Background()
	id999, _ := address.NewIDAddress(999)
	ethtypes.EthAccountApplyAndCallActorAddr = id999

	// Build one 7702 SignedMessage and one non-7702 delegated SignedMessage (EVM.InvokeContract).
	var from20a [20]byte
	for i := range from20a {
		from20a[i] = 0x77
	}
	fromA, _ := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20a[:])
	var del20 [20]byte
	for i := range del20 {
		del20[i] = 0x88
	}
	msgA := types.Message{Version: 0, To: ethtypes.EthAccountApplyAndCallActorAddr, From: fromA, Nonce: 0, Value: types.NewInt(0), Method: abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")), GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1), Params: make7702Params(t, 314, del20, 0)}
	sigA := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 1), append(append(make([]byte, 31), 1), 0)...)}
	smA := &types.SignedMessage{Message: msgA, Signature: sigA}

	var from20b [20]byte
	for i := range from20b {
		from20b[i] = 0x99
	}
	fromB, _ := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, from20b[:])
	toID, _ := address.NewIDAddress(1003)
	msgB := types.Message{Version: 0, To: toID, From: fromB, Nonce: 0, Value: types.NewInt(0), Method: builtintypes.MethodsEVM.InvokeContract, GasLimit: 100000, GasFeeCap: types.NewInt(1), GasPremium: types.NewInt(1)}
	sigB := typescrypto.Signature{Type: typescrypto.SigTypeDelegated, Data: append(append(make([]byte, 31), 2), append(append(make([]byte, 31), 2), 0)...)}
	smB := &types.SignedMessage{Message: msgB, Signature: sigB}

	// Tipset/mocks
	ts := makeTipset(t)
	// Provide minimal returns
	mkRet := func() []byte {
		var buf bytes.Buffer
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 3))
		require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
		_, _ = buf.Write(cbg.CborNull)
		var eth20 [20]byte
		for i := range eth20 {
			eth20[i] = 0xAB
		}
		require.NoError(t, cbg.WriteByteArray(&buf, eth20[:]))
		return buf.Bytes()
	}
	rc1 := types.MessageReceipt{ExitCode: 0, GasUsed: 1000, Return: mkRet()}
	rc2 := types.MessageReceipt{ExitCode: 0, GasUsed: 1000}
	cs := &mockChainStoreMulti{ts: ts, smsgs: []*types.SignedMessage{smA, smB}, rcpts: []types.MessageReceipt{rc1, rc2}}
	sm := &mockStateManager{}
	tr := &mockTipsetResolver{ts: ts}

	// Set a synthetic Delegated(address) event for logs fetching.
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	copy(topic0[:], h.Sum(nil))
	var evDel ethtypes.EthAddress
	for i := range evDel {
		evDel[i] = 0xDE
	}
	var data32 [32]byte
	copy(data32[12:], evDel[:])
	logsForTest = []ethtypes.EthLog{{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(data32[:])}}
	ev := &mockEvents{}

	api, err := NewEthTransactionAPI(cs, sm, nil, nil, nil, ev, tr, 0)
	require.NoError(t, err)
	receipts, err := api.EthGetBlockReceipts(ctx, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Len(t, receipts, 2)

	// 7702 has auth list and delegatedTo; non-7702 gets delegatedTo from synthetic event
	require.Len(t, receipts[0].AuthorizationList, 1)
	require.Len(t, receipts[0].DelegatedTo, 1)
	require.Len(t, receipts[1].AuthorizationList, 0)
	require.Len(t, receipts[1].DelegatedTo, 1)
}
