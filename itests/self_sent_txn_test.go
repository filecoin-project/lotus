package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// these tests check that the versioned code in vm.transfer is functioning correctly across versions!
// we reordered the checks to make sure that a transaction with too much money in it sent to yourself will fail instead of succeeding as a noop
// more info in this PR! https://github.com/filecoin-project/lotus/pull/7637
func TestSelfSentTxnV15(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client15, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version15))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	bal, err := client15.WalletBalance(ctx, client15.DefaultKey.Address)
	require.NoError(t, err)

	// send self half of account balance
	msgHalfBal := &types.Message{
		From:  client15.DefaultKey.Address,
		To:    client15.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	smHalfBal, err := client15.MpoolPushMessage(ctx, msgHalfBal, nil)
	require.NoError(t, err)
	mLookup, err := client15.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	msgOverBal := &types.Message{
		From:       client15.DefaultKey.Address,
		To:         client15.DefaultKey.Address,
		Value:      big.Mul(big.NewInt(2), bal),
		GasLimit:   10000000000,
		GasPremium: big.NewInt(10000000000),
		GasFeeCap:  big.NewInt(100000000000),
		Nonce:      1,
	}
	smOverBal, err := client15.WalletSignMessage(ctx, client15.DefaultKey.Address, msgOverBal)
	require.NoError(t, err)
	smcid, err := client15.MpoolPush(ctx, smOverBal)
	require.NoError(t, err)
	mLookup, err = client15.StateWaitMsg(ctx, smcid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.SysErrInsufficientFunds, mLookup.Receipt.ExitCode)
}

func TestSelfSentTxnV14(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client14, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version14))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	bal, err := client14.WalletBalance(ctx, client14.DefaultKey.Address)
	require.NoError(t, err)

	// send self half of account balance
	msgHalfBal := &types.Message{
		From:  client14.DefaultKey.Address,
		To:    client14.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	smHalfBal, err := client14.MpoolPushMessage(ctx, msgHalfBal, nil)
	require.NoError(t, err)
	mLookup, err := client14.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	msgOverBal := &types.Message{
		From:       client14.DefaultKey.Address,
		To:         client14.DefaultKey.Address,
		Value:      big.Mul(big.NewInt(2), bal),
		GasLimit:   10000000000,
		GasPremium: big.NewInt(10000000000),
		GasFeeCap:  big.NewInt(100000000000),
		Nonce:      1,
	}
	smOverBal, err := client14.WalletSignMessage(ctx, client14.DefaultKey.Address, msgOverBal)
	require.NoError(t, err)
	smcid, err := client14.MpoolPush(ctx, smOverBal)
	require.NoError(t, err)
	mLookup, err = client14.StateWaitMsg(ctx, smcid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)
}
