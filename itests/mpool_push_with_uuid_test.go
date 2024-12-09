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

func TestMpoolPushWithoutUuidWithMaxFee(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client15, _, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	bal, err := client15.WalletBalance(ctx, client15.DefaultKey.Address)
	require.NoError(t, err)

	// send self half of account balance
	msgHalfBal := &types.Message{
		From:  client15.DefaultKey.Address,
		To:    client15.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	smHalfBal, err := client15.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{MaxFee: abi.TokenAmount(config.DefaultDefaultMaxFee())})
	require.NoError(t, err)
	mLookup, err := client15.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	msgQuarterBal := &types.Message{
		From:  client15.DefaultKey.Address,
		To:    client15.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(4)),
	}
	smcid, err := client15.MpoolPushMessage(ctx, msgQuarterBal, &api.MessageSendSpec{MaxFee: abi.TokenAmount(config.DefaultDefaultMaxFee())})
	require.NoError(t, err)
	mLookup, err = client15.StateWaitMsg(ctx, smcid.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)
}
