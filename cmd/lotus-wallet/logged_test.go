package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/sigs"
)

func TestLoggedWalletSignChainMsg(t *testing.T) {
	ctx := context.Background()

	lw, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	w := &LoggedWallet{under: lw}

	secpAddr, err := lw.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)
	delegatedAddr, err := lw.WalletNew(ctx, types.KTDelegated)
	require.NoError(t, err)
	idAddr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		msg  *types.Message
	}{
		{
			name: "secp256k1",
			msg: &types.Message{
				From:       secpAddr,
				To:         idAddr,
				Value:      big.NewInt(1),
				GasLimit:   1000,
				GasFeeCap:  big.NewInt(1),
				GasPremium: big.NewInt(1),
			},
		},
		{
			// Delegated senders sign the RLP-encoded Ethereum transaction, not
			// the message CID.
			name: "delegated",
			msg: &types.Message{
				From:       delegatedAddr,
				To:         builtin.EthereumAddressManagerActorAddr,
				Method:     builtin.MethodsEAM.CreateExternal,
				Params:     []byte{0x41, 0x00},
				Value:      big.Zero(),
				GasLimit:   1000,
				GasFeeCap:  big.NewInt(1),
				GasPremium: big.NewInt(1),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := tc.msg.From
			sb, err := messagesigner.SigningBytes(tc.msg, k.Protocol())
			require.NoError(t, err)
			mb, err := tc.msg.ToStorageBlock()
			require.NoError(t, err)
			meta := api.MsgMeta{Type: api.MTChainMsg, Extra: mb.RawData()}

			sig, err := w.WalletSign(ctx, k, sb, meta)
			require.NoError(t, err)
			require.NoError(t, sigs.Verify(sig, k, sb))

			for _, toSign := range [][]byte{
				append(append([]byte{}, sb...), "suffix"...),
				sb[:len(sb)-1],
			} {
				_, err := w.WalletSign(ctx, k, toSign, meta)
				require.ErrorContains(t, err, "signing bytes do not match")
			}
		})
	}
}
