package messagepool

import (
    "context"
    "testing"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/stretchr/testify/require"

    "github.com/filecoin-project/go-address"
    builtintypes "github.com/filecoin-project/go-state-types/builtin"
    "github.com/filecoin-project/go-state-types/crypto"
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

