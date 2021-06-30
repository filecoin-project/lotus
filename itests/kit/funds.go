package kit

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

// SendFunds sends funds from the default wallet of the specified sender node
// to the recipient address.
func SendFunds(ctx context.Context, t *testing.T, sender *TestFullNode, recipient address.Address, amount abi.TokenAmount) {
	senderAddr, err := sender.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	msg := &types.Message{
		From:  senderAddr,
		To:    recipient,
		Value: amount,
	}

	sm, err := sender.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := sender.StateWaitMsg(ctx, sm.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	require.EqualValues(t, 0, res.Receipt.ExitCode, "did not successfully send funds")
}
