package itests

import (
	"context"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDuplicateMpoolMessages(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(blockTime)

	// send f099 half of account balance
	msgBal := &types.Message{
		From:  client.DefaultKey.Address,
		To:    builtin.BurntFundsActorAddr,
		Value: big.NewInt(10000),
	}

	uuid := uuid2.New()
	msgSpec := &api.MessageSendSpec{MsgUuid: uuid}

	msg, err := client.MpoolPushMessage(ctx, msgBal, msgSpec)
	require.NoError(t, err)

	client.StateWaitMsg(ctx, msg.Cid(), 3, api.LookbackNoLimit, true)

	remBal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	msg2, err := client.MpoolPushMessage(ctx, msgBal, msgSpec)
	require.NoError(t, err)

	currBal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	require.Equal(t, msg, msg2)
	require.Equal(t, remBal, currBal)
}
