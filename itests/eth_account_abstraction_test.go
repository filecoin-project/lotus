package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEthAccountAbstraction goes over the account abstraction workflow:
// - an embryo is created when it receives a message
// - the embryo turns into an EOA when it sends a message
func TestEthAccountAbstraction(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	embryoAddress, err := address.NewFromString("t410fhreuqiy6apawwjpysfdqo2typwbb3ssfqfapmca")
	require.NoError(t, err)

	// create an embryo actor at the target address
	msgCreateEmbryo := &types.Message{
		From:  client.DefaultKey.Address,
		To:    embryoAddress,
		Value: abi.TokenAmount(types.MustParseFIL("100")),
	}
	smCreateEmbryo, err := client.MpoolPushMessage(ctx, msgCreateEmbryo, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, smCreateEmbryo.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// confirm the embryo is an embryo
	embryoActor, err := client.StateGetActor(ctx, embryoAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.True(t, builtin.IsEmbryo(embryoActor.Code))

	// send a message from the embryo address
	msgFromEmbryo := &types.Message{
		From: embryoAddress,
		// self-send because an "eth tx payload" can't be to a filecoin address?
		To: embryoAddress,
	}
	msgFromEmbryo, err = client.GasEstimateMessageGas(ctx, msgFromEmbryo, nil, types.EmptyTSK)
	require.NoError(t, err)

	smFromEmbryo := &types.SignedMessage{
		Message:   *msgFromEmbryo,
		Signature: crypto.Signature{Type: crypto.SigTypeDelegated},
	}

	// TODO: Hack delegated verification to always be true

	fmt.Println(smFromEmbryo.Message.From)
	_, err = client.MpoolPush(ctx, smFromEmbryo)
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromEmbryo.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// confirm ugly embryo duckling has turned into a beautiful EOA swan

	eoaActor, err := client.StateGetActor(ctx, embryoAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.False(t, builtin.IsEmbryo(eoaActor.Code))
	// TODO: Uncomment when method exists
	//require.True(t, builtin.IsEOAActor(embryoActor.Code))
}
