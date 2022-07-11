package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/types"
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

	_, err = client.StateWaitMsg(ctx, sm.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

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
