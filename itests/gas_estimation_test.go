package itests

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
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

// Make sure that we correctly calculate the inclusion cost. Especially, make sure the FVM and Lotus
// agree and that:
//  1. The FVM will never charge _less_ than the inclusion cost.
//  2. The FVM will never fine a storage provider for including a message that costs exactly the
//     inclusion cost.
func TestEstimateInclusion(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	// We need this to be "correct" in this test so that lotus can get the correct gas value
	// (which, unfortunately, looks at the height and not the current network version).
	oldPrices := vm.Prices
	vm.Prices = map[abi.ChainEpoch]vm.Pricelist{
		0: oldPrices[buildconstants.UpgradeHyggeHeight],
	}
	t.Cleanup(func() { vm.Prices = oldPrices })

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// First, try sending a message that should have no fees beyond the inclusion cost. I.e., it
	// does absolutely nothing:
	msg := &types.Message{
		From:       client.DefaultKey.Address,
		To:         client.DefaultKey.Address,
		Value:      big.Zero(),
		GasLimit:   0,
		GasFeeCap:  abi.NewTokenAmount(10000),
		GasPremium: big.Zero(),
	}

	burntBefore, err := client.WalletBalance(ctx, builtin.BurntFundsActorAddr)
	require.NoError(t, err)
	balanceBefore, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

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

	// Then try sending a message of the same size that tries to create an actor. This should
	// get successfully included, but fail with out of gas:

	// Mutate the last byte to get a new address of the same length.
	toBytes := msg.To.Bytes()
	toBytes[len(toBytes)-1] += 1 // revive:disable-line:increment-decrement
	newAddr, err := address.NewFromBytes(toBytes)
	require.NoError(t, err)

	msg.Nonce = 1
	msg.To = newAddr
	smsg, err = client.WalletSignMessage(ctx, client.DefaultKey.Address, msg)
	require.NoError(t, err)

	cid, err = client.MpoolPush(ctx, smsg)
	require.NoError(t, err)
	ret, err = client.StateWaitMsg(ctx, cid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, ret.Receipt.ExitCode, exitcode.SysErrOutOfGas)
	require.Equal(t, msg.GasLimit, ret.Receipt.GasUsed)

	// Now make sure that the client is the only contributor to the burnt funds actor (the
	// miners should not have been fined for either message).

	burntAfter, err := client.WalletBalance(ctx, builtin.BurntFundsActorAddr)
	require.NoError(t, err)
	balanceAfter, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	burnt := big.Sub(burntAfter, burntBefore)
	spent := big.Sub(balanceBefore, balanceAfter)

	require.Equal(t, burnt, spent)

	// Finally, try to submit a message with too little gas. This should fail.

	msg.Nonce = 2
	msg.To = msg.From
	msg.GasLimit -= 1 // revive:disable-line:increment-decrement

	smsg, err = client.WalletSignMessage(ctx, client.DefaultKey.Address, msg)
	require.NoError(t, err)

	_, err = client.MpoolPush(ctx, smsg)
	require.ErrorContains(t, err, "will not be included in a block")
	require.ErrorContains(t, err, "cannot be less than the cost of storing a message")
}
