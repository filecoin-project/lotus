package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEthAccountAbstraction goes over the account abstraction workflow:
// - an placeholder is created when it receives a message
// - the placeholder turns into an EOA when it sends a message
func TestEthAccountAbstraction(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpKey, err := key.GenerateKey(types.KTDelegated)
	require.NoError(t, err)

	placeholderAddress, err := client.WalletImport(ctx, &secpKey.KeyInfo)
	require.NoError(t, err)

	fmt.Println(placeholderAddress)

	// create an placeholder actor at the target address
	msgCreatePlaceholder := &types.Message{
		From:  client.DefaultKey.Address,
		To:    placeholderAddress,
		Value: abi.TokenAmount(types.MustParseFIL("100")),
	}
	smCreatePlaceholder, err := client.MpoolPushMessage(ctx, msgCreatePlaceholder, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, smCreatePlaceholder.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// confirm the placeholder is an placeholder
	placeholderActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, uint64(0), placeholderActor.Nonce)
	require.True(t, builtin.IsPlaceholderActor(placeholderActor.Code))

	// send a message from the placeholder address
	msgFromPlaceholder := &types.Message{
		From: placeholderAddress,
		// self-send because an "eth tx payload" can't be to a filecoin address?
		To: placeholderAddress,
	}
	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	txArgs, err := ethtypes.NewEthTxArgsFromMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err := txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err := client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err := client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// confirm ugly Placeholder duckling has turned into a beautiful EthAccount swan

	eoaActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
	require.Equal(t, uint64(1), eoaActor.Nonce)

	// Send another message, it should succeed without any code CID changes

	msgFromPlaceholder = &types.Message{
		From:  placeholderAddress,
		To:    placeholderAddress,
		Nonce: 1,
	}

	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	txArgs, err = ethtypes.NewEthTxArgsFromMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err = txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err = client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err = client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// confirm no changes in code CID

	eoaActor, err = client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, uint64(2), eoaActor.Nonce)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
}
