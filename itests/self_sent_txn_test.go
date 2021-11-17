package itests

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/network"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestSelfSentTxn(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version15))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// send self half of account balance
	msgExactlyBal := &types.Message{
		From:  client.DefaultKey.Address,
		To:    client.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	smExactlyBal, err := client.MpoolPushMessage(ctx, msgExactlyBal, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, smExactlyBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	msgOverBal := &types.Message{
		From:  client.DefaultKey.Address,
		To:    client.DefaultKey.Address,
		Value: big.Mul(big.NewInt(2), bal),
	}
	_, err = client.MpoolPushMessage(ctx, msgOverBal, nil)
	require.Error(t, err)
	// this is horrifying and a crime... I don't know how else to check it.
	require.Contains(t, fmt.Sprintf("%v", err), "SysErrInsufficientFunds")
	//mLookup, err = client.StateWaitMsg(ctx, smOverBal.Cid(), 3, api.LookbackNoLimit, true)
	//require.NoError(t, err)
	//require.Equal(t, exitcode.SysErrInsufficientFunds, mLookup.Receipt.ExitCode)
}
