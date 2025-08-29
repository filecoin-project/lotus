package messagepool

import (
    "bytes"
    "context"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"

    "github.com/filecoin-project/go-address"
    builtintypes "github.com/filecoin-project/go-state-types/builtin"
    "github.com/filecoin-project/go-state-types/crypto"
    "github.com/filecoin-project/go-state-types/network"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/build/buildconstants"
    delegator "github.com/filecoin-project/lotus/chain/actors/builtin/delegator"
    "github.com/filecoin-project/lotus/chain/types"
    ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
    "github.com/filecoin-project/lotus/chain/wallet"
)

// encode a single 7702 tuple as CBOR array-of-arrays
func encodeSingleTuple(t *testing.T, chainID uint64, addr20 [20]byte, nonce uint64) []byte {
    t.Helper()
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 1))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, chainID))
    require.NoError(t, cbg.WriteByteArray(&buf, addr20[:]))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, nonce))
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0)) // y_parity
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))               // r
    require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))               // s
    return buf.Bytes()
}

func signMsg(t *testing.T, w *wallet.LocalWallet, msg *types.Message) *types.SignedMessage {
    t.Helper()
    sig, err := w.WalletSign(context.TODO(), msg.From, msg.Cid().Bytes(), api.MsgMeta{})
    require.NoError(t, err)
    return &types.SignedMessage{Message: *msg, Signature: *sig}
}

func TestCrossAccountInvalidation_Applies(t *testing.T) {
    mp, tma := makeTestMpool()
    // Ensure NV is at/after activation for tests
    tma.nv = buildconstants.TestNetworkVersion

    // Create a delegated authority (f4) with some pending message at nonce 5
    wAuth, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    authAddr, err := wAuth.WalletNew(context.Background(), types.KTDelegated)
    require.NoError(t, err)
    tma.setBalance(authAddr, 10)

    // Add a pending message from authority at nonce 5
    msgAuth := &types.Message{
        From:       authAddr,
        To:         authAddr, // self, arbitrary
        Method:     0,
        Value:      types.NewInt(0),
        Nonce:      5,
        GasLimit:   1000000,
        GasFeeCap:  types.NewInt(100),
        GasPremium: types.NewInt(1),
    }
    smAuth := signMsg(t, wAuth, msgAuth)
    require.NoError(t, mp.Add(context.Background(), smAuth))

    // Configure DelegatorActorAddr and build a 7702 ApplyDelegations message targeting the authority nonce 5
    ethtypes.DelegatorActorAddr, _ = address.NewIDAddress(1234)
    // authority Eth 20 bytes from f4 delegated address
    ethAuth, err := ethtypes.EthAddressFromFilecoinAddress(authAddr)
    require.NoError(t, err)
    params := encodeSingleTuple(t, uint64(buildconstants.Eip155ChainId), ethAuth, 5)

    // Sender signs the delegation apply message
    wSender, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    sender, _ := wSender.WalletNew(context.Background(), types.KTSecp256k1)
    tma.setBalance(sender, 10)
    msgDel := &types.Message{
        From:       sender,
        To:         ethtypes.DelegatorActorAddr,
        Method:     delegator.MethodApplyDelegations,
        Value:      types.NewInt(0),
        Nonce:      0,
        GasLimit:   1000000,
        GasFeeCap:  types.NewInt(100),
        GasPremium: types.NewInt(1),
        Params:     params,
    }
    smDel := signMsg(t, wSender, msgDel)

    // Enable feature for invalidation hook
    ethtypes.Eip7702FeatureEnabled = true
    defer func() { ethtypes.Eip7702FeatureEnabled = false }()

    // Push delegation message; this should evict authority's nonce 5 message
    require.NoError(t, mp.Add(context.Background(), smDel))

    // Verify eviction
    ms, ok, err := mp.getPendingMset(context.Background(), authAddr)
    require.NoError(t, err)
    if ok {
        if _, exists := ms.msgs[5]; exists {
            t.Fatalf("expected authority nonce 5 to be evicted")
        }
    }
}

func TestCrossAccountInvalidation_MultiAuthority(t *testing.T) {
    mp, tma := makeTestMpool()
    tma.nv = buildconstants.TestNetworkVersion

    // two authorities
    wA, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    a1, _ := wA.WalletNew(context.Background(), types.KTDelegated)
    wB, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    a2, _ := wB.WalletNew(context.Background(), types.KTDelegated)
    tma.setBalance(a1, 10)
    tma.setBalance(a2, 10)

    // pending messages for both at nonce 1
    add := func(w *wallet.LocalWallet, from address.Address) {
        m := &types.Message{From: from, To: from, Method: 0, Value: types.NewInt(0), Nonce: 1, GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1)}
        require.NoError(t, mp.Add(context.Background(), signMsg(t, w, m)))
    }
    add(wA, a1)
    add(wB, a2)

    // Build params with two tuples
    var buf bytes.Buffer
    require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
    encTup := func(addr20 [20]byte, nonce uint64) {
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 6))
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(buildconstants.Eip155ChainId)))
        require.NoError(t, cbg.WriteByteArray(&buf, addr20[:]))
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, nonce))
        require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
        require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
        require.NoError(t, cbg.WriteByteArray(&buf, []byte{1}))
    }
    e1, _ := ethtypes.EthAddressFromFilecoinAddress(a1)
    e2, _ := ethtypes.EthAddressFromFilecoinAddress(a2)
    encTup(e1, 1)
    encTup(e2, 1)

    ethtypes.DelegatorActorAddr, _ = address.NewIDAddress(1234)
    // sender
    wS, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    s, _ := wS.WalletNew(context.Background(), types.KTSecp256k1)
    tma.setBalance(s, 10)
    del := &types.Message{From: s, To: ethtypes.DelegatorActorAddr, Method: delegator.MethodApplyDelegations, Value: types.NewInt(0), Nonce: 0, GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1), Params: buf.Bytes()}
    ethtypes.Eip7702FeatureEnabled = true
    defer func() { ethtypes.Eip7702FeatureEnabled = false }()
    require.NoError(t, mp.Add(context.Background(), signMsg(t, wS, del)))

    // both authorities should be evicted at nonce 1
    checkGone := func(addr address.Address) {
        if ms, ok, err := mp.getPendingMset(context.Background(), addr); err == nil && ok {
            if _, ex := ms.msgs[1]; ex {
                t.Fatalf("expected eviction for %s", addr)
            }
        }
    }
    checkGone(a1)
    checkGone(a2)
}

func TestDelegationCap_Enforced(t *testing.T) {
    mp, tma := makeTestMpool()
    tma.nv = buildconstants.TestNetworkVersion

    // sender and delegator addr configured
    wS, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    s, _ := wS.WalletNew(context.Background(), types.KTSecp256k1)
    tma.setBalance(s, 10)
    ethtypes.DelegatorActorAddr, _ = address.NewIDAddress(1234)

    // add 4 pending ApplyDelegations from the same sender
    addDel := func(nonce uint64) error {
        m := &types.Message{From: s, To: ethtypes.DelegatorActorAddr, Method: delegator.MethodApplyDelegations, Value: types.NewInt(0), Nonce: nonce, GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1)}
        return mp.Add(context.Background(), signMsg(t, wS, m))
    }
    require.NoError(t, addDel(0))
    require.NoError(t, addDel(1))
    require.NoError(t, addDel(2))
    require.NoError(t, addDel(3))

    // 5th should be rejected
    require.Error(t, addDel(4))
}

func TestCrossAccountInvalidation_DisabledBeforeActivation(t *testing.T) {
    mp, tma := makeTestMpool()
    // Set network version below activation
    tma.nv = network.Version26

    // Authority with pending nonce 1
    wA, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    a1, _ := wA.WalletNew(context.Background(), types.KTDelegated)
    tma.setBalance(a1, 10)
    m := &types.Message{From: a1, To: a1, Method: 0, Value: types.NewInt(0), Nonce: 1, GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1)}
    require.NoError(t, mp.Add(context.Background(), signMsg(t, wA, m)))

    // ApplyDelegations should NOT evict before activation
    ethtypes.DelegatorActorAddr, _ = address.NewIDAddress(1234)
    e1, _ := ethtypes.EthAddressFromFilecoinAddress(a1)
    params := encodeSingleTuple(t, uint64(buildconstants.Eip155ChainId), e1, 1)

    wS, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    s, _ := wS.WalletNew(context.Background(), types.KTSecp256k1)
    tma.setBalance(s, 10)
    del := &types.Message{From: s, To: ethtypes.DelegatorActorAddr, Method: delegator.MethodApplyDelegations, Value: types.NewInt(0), Nonce: 0, GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1), Params: params}
    require.NoError(t, mp.Add(context.Background(), signMsg(t, wS, del)))

    // Verify original message still present
    ms, ok, err := mp.getPendingMset(context.Background(), a1)
    require.NoError(t, err)
    require.True(t, ok)
    if _, ex := ms.msgs[1]; !ex {
        t.Fatalf("did not expect eviction before activation")
    }
}

func TestDelegationCap_NotEnforcedBeforeActivation(t *testing.T) {
    mp, tma := makeTestMpool()
    tma.nv = network.Version26

    wS, _ := wallet.NewWallet(wallet.NewMemKeyStore())
    s, _ := wS.WalletNew(context.Background(), types.KTSecp256k1)
    tma.setBalance(s, 10)
    ethtypes.DelegatorActorAddr, _ = address.NewIDAddress(1234)

    // Add more than 4 without rejection
    for i := 0; i < 5; i++ {
        m := &types.Message{From: s, To: ethtypes.DelegatorActorAddr, Method: delegator.MethodApplyDelegations, Value: types.NewInt(0), Nonce: uint64(i), GasLimit: 1000000, GasFeeCap: types.NewInt(100), GasPremium: types.NewInt(1)}
        require.NoError(t, mp.Add(context.Background(), signMsg(t, wS, m)))
    }

    // Count delegation messages from sender should be 5
    ms, ok, err := mp.getPendingMset(context.Background(), s)
    require.NoError(t, err)
    require.True(t, ok)
    count := 0
    for _, pm := range ms.msgs {
        if pm.Message.To == ethtypes.DelegatorActorAddr && pm.Message.Method == delegator.MethodApplyDelegations {
            count++
        }
    }
    require.Equal(t, 5, count)
}
