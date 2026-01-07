package eth

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// Ensures that for typed-0x04 ApplyAndCall, we decode embedded status from return payload
func TestNewEthTxReceipt_7702_StatusFromApplyAndCallReturn(t *testing.T) {
	ctx := context.Background()
	// Build a minimal 0x04 EthTx view
	// Non-creation tx (To != nil)
	var to ethtypes.EthAddress
	for i := range to {
		to[i] = 0x11
	}
	tx := ethtypes.EthTx{Type: 0x04, Gas: 21000, To: &to}
	// Minimal fee fields so newEthTxReceipt can compute gas outputs
	one := big.NewInt(1)
	tx.MaxFeePerGas = &ethtypes.EthBigInt{Int: one.Int}
	tx.MaxPriorityFeePerGas = &ethtypes.EthBigInt{Int: one.Int}

	// Encode ApplyAndCallReturn { status=0, output_data=[0xAA] }
	var buf bytes.Buffer
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 0))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{0xAA}))

	msgReceipt := types.MessageReceipt{ExitCode: 0, GasUsed: 21000, Return: buf.Bytes()}
	rcpt, err := newEthTxReceipt(ctx, tx, big.Zero(), msgReceipt, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(0), uint64(rcpt.Status))

	// Now test status=1
	buf.Reset()
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajArray, 2))
	require.NoError(t, cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, 1))
	require.NoError(t, cbg.WriteByteArray(&buf, []byte{}))
	msgReceipt.Return = buf.Bytes()
	rcpt, err = newEthTxReceipt(ctx, tx, big.Zero(), msgReceipt, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(1), uint64(rcpt.Status))
}
