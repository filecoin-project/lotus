package kit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

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

	WaitMsg(ctx, t, sender, sm.Cid())
}

func WaitMsg(ctx context.Context, t *testing.T, node *TestFullNode, msg cid.Cid) {
	res, err := node.StateWaitMsg(ctx, msg, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	require.EqualValues(t, 0, res.Receipt.ExitCode, "message did not successfully execute")
}
