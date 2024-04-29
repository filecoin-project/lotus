package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
)

func TestMsgWithoutUuidWithMaxFee(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	node, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	bal, err := node.WalletBalance(ctx, node.DefaultKey.Address)
	require.NoError(t, err)

	// send self half of account balance
	msgHalfBal := &types.Message{
		From:  node.DefaultKey.Address,
		To:    node.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	smHalfBal, err := node.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{MaxFee: abi.TokenAmount(config.DefaultDefaultMaxFee())})
	require.NoError(t, err)
	mLookup, err := node.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	msgQuarterBal := &types.Message{
		From:  node.DefaultKey.Address,
		To:    node.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(4)),
	}
	smQuarterBal, err := node.MpoolPushMessage(ctx, msgQuarterBal, &api.MessageSendSpec{MaxFee: abi.TokenAmount(config.DefaultDefaultMaxFee())})
	require.NoError(t, err)

	require.Equal(t, msgQuarterBal.Value, smQuarterBal.Message.Value)
	mLookup, err = node.StateWaitMsg(ctx, smQuarterBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)
}
