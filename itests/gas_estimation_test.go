package itests

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestEstimateGasNoFunds(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// create a new address
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	// Create that address.
	msg := &types.Message{
		From:  client.DefaultKey.Address,
		To:    addr,
		Value: big.Zero(),
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	ret, err := client.StateWaitMsg(ctx, sm.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, ret.Receipt.ExitCode.IsSuccess())

	// Make sure we can estimate gas even if we have no funds.
	msg2 := &types.Message{
		From:   addr,
		To:     client.DefaultKey.Address,
		Method: account.Methods.PubkeyAddress,
		Value:  big.Zero(),
	}

	limit, err := client.GasEstimateGasLimit(ctx, msg2, types.EmptyTSK)
	require.NoError(t, err)
	require.NotZero(t, limit)
}

func TestEstimateNoop(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)
	msg := &types.Message{
		From:       client.DefaultKey.Address,
		To:         client.DefaultKey.Address,
		Value:      big.Zero(),
		GasLimit:   0,
		GasFeeCap:  abi.NewTokenAmount(10000),
		GasPremium: abi.NewTokenAmount(10000),
	}

	// Sign the message and compute the correct inclusion cost.
	var smsg *types.SignedMessage
	for i := 0; ; i++ {
		var err error
		smsg, err = client.WalletSignMessage(ctx, client.DefaultKey.Address, msg)
		require.NoError(t, err)
		estimatedGas := vm.PricelistByEpoch(math.MaxInt).OnChainMessage(smsg.ChainLength()).Total()
		if estimatedGas == msg.GasLimit {
			break
		}
		// Try 10 times to get the right gas value.
		require.Less(t, i, 10, "unable to estimate gas: %s != %s", estimatedGas, msg.GasLimit)
		msg.GasLimit = estimatedGas
	}

	cid, err := client.MpoolPush(ctx, smsg)
	require.NoError(t, err)
	ret, err := client.StateWaitMsg(ctx, cid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, ret.Receipt.ExitCode.IsSuccess())
	require.Equal(t, msg.GasLimit, ret.Receipt.GasUsed)
}
