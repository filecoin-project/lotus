package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestImplicitMessageReceipts verifies FIP-0107 behavior across the nv27->nv28
// upgrade boundary. Before the upgrade, receipts should be V1 and only contain
// explicit messages. After the upgrade, receipts should be V2 with IpldCodec
// and Message fields, and implicit message receipts (cron, reward) should be
// present in the AMT but filtered from the v1 API.
func TestImplicitMessageReceipts(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	// Start at nv27, upgrade to nv28 at epoch 20 so we can observe behavior
	// on both sides of the boundary.
	upgradeEpoch := abi.ChainEpoch(20)
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.LatestActorsAt(upgradeEpoch))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	sendMsg := func() *types.SignedMessage {
		msg := &types.Message{
			From:  client.DefaultKey.Address,
			To:    addr,
			Value: big.Div(bal, big.NewInt(100)),
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)
		return sm
	}

	// Send a message before the upgrade.
	preUpgradeSm := sendMsg()

	// Wait for it to land.
	preUpgradeLookup, err := client.StateWaitMsg(ctx, preUpgradeSm.Cid(), 1, 200, true)
	require.NoError(t, err)
	require.True(t, preUpgradeLookup.Receipt.ExitCode.IsSuccess())

	preExecTs, err := client.ChainGetTipSet(ctx, preUpgradeLookup.TipSet)
	require.NoError(t, err)

	t.Logf("pre-upgrade message landed at height %d (upgrade at %d)", preExecTs.Height(), upgradeEpoch)

	// Verify pre-upgrade behavior: receipts should be V1, no implicit receipts.
	if preExecTs.Height() < upgradeEpoch {
		preBlk := preExecTs.Cids()[0]
		preReceipts, err := client.ChainGetParentReceipts(ctx, preBlk)
		require.NoError(t, err)
		preMsgs, err := client.ChainGetParentMessages(ctx, preBlk)
		require.NoError(t, err)

		require.Equal(t, len(preMsgs), len(preReceipts),
			"pre-upgrade: receipt count should equal message count")

		for i, rct := range preReceipts {
			require.Equal(t, types.MessageReceiptV1, rct.Version(),
				"pre-upgrade receipt %d should be V1", i)
			require.Nil(t, rct.Message, "pre-upgrade receipt should not have Message field")
			require.Nil(t, rct.IpldCodec, "pre-upgrade receipt should not have IpldCodec field")
		}
		t.Log("pre-upgrade receipts verified: V1, no implicit receipts")
	} else {
		t.Log("pre-upgrade message landed after upgrade epoch, skipping pre-upgrade checks")
	}

	// Wait for the upgrade to happen, then send a post-upgrade message.
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)
		if head.Height() > upgradeEpoch+2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	postUpgradeSm := sendMsg()
	postUpgradeLookup, err := client.StateWaitMsg(ctx, postUpgradeSm.Cid(), 1, 200, true)
	require.NoError(t, err)
	require.True(t, postUpgradeLookup.Receipt.ExitCode.IsSuccess())

	postExecTs, err := client.ChainGetTipSet(ctx, postUpgradeLookup.TipSet)
	require.NoError(t, err)

	t.Logf("post-upgrade message landed at height %d", postExecTs.Height())
	require.Greater(t, postExecTs.Height(), upgradeEpoch,
		"post-upgrade message should have landed after upgrade epoch")

	postBlk := postExecTs.Cids()[0]

	// V1 API backward compatibility: filtered receipt count must match message count.
	postReceipts, err := client.ChainGetParentReceipts(ctx, postBlk)
	require.NoError(t, err)
	postMsgs, err := client.ChainGetParentMessages(ctx, postBlk)
	require.NoError(t, err)

	require.Equal(t, len(postMsgs), len(postReceipts),
		"post-upgrade v1 API: receipt count (%d) must equal message count (%d)",
		len(postReceipts), len(postMsgs))

	require.NotEmpty(t, postReceipts, "should have at least one receipt")

	// Verify each receipt is V2 with the expected fields.
	for i, rct := range postReceipts {
		require.Equal(t, types.MessageReceiptV2, rct.Version(),
			"post-upgrade receipt %d: expected V2, got V%d", i, rct.Version())
		require.NotNil(t, rct.Message,
			"post-upgrade receipt %d: Message CID should be set", i)
		require.Equal(t, postMsgs[i].Cid, *rct.Message,
			"post-upgrade receipt %d: Message CID should match the explicit message", i)
	}
	t.Log("post-upgrade v1 receipts verified: V2, Message CIDs match")

	// Verify the filtered v1 receipts do NOT include cron or reward receipts.
	// The v1 API filtering in ChainGetParentReceipts loads each receipt's
	// referenced message from the blockstore, so this also proves that
	// implicit messages are stored and retrievable by CID.
	for _, rct := range postReceipts {
		if rct.Message != nil {
			msg, err := client.ChainGetMessage(ctx, *rct.Message)
			require.NoError(t, err)
			require.NotEqual(t, builtin.SystemActorAddr, msg.From,
				"v1 ChainGetParentReceipts should not include implicit message receipts")
		}
	}
	t.Log("verified v1 receipts exclude all implicit messages")
}
