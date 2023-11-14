// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

const mPoolThrottle = time.Millisecond * 100
const mPoolTimeout = time.Second * 10

func TestMemPoolPushSingleNode(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_CREATE_MSG_CHAINS_001, @CHAIN_MEMPOOL_SELECT_001
	//stm: @CHAIN_MEMPOOL_PENDING_001, @CHAIN_STATE_WAIT_MSG_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	//stm: @CHAIN_MEMPOOL_PUSH_002
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, _, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address

	addr, err := firstNode.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	const totalMessages = 10

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)
	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	// add messages to be mined/published
	var sms []*types.SignedMessage
	for i := 0; i < totalMessages; i++ {
		msg := &types.Message{
			From:  sender,
			To:    addr,
			Value: each,
		}

		sm, err := firstNode.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)
		require.EqualValues(t, i, sm.Message.Nonce)

		sms = append(sms, sm)
	}

	// check pending messages for address
	kit.CircuitBreaker(t, "push messages", mPoolThrottle, mPoolTimeout, func() bool {
		msgStatuses, _ := firstNode.MpoolCheckPendingMessages(ctx, sender)
		if len(msgStatuses) == totalMessages {
			for _, msgStatusList := range msgStatuses {
				for _, status := range msgStatusList {
					require.True(t, status.OK)
				}
			}
			return true
		}
		return false
	})

	// verify messages should be the ones included in the next block
	selected, _ := firstNode.MpoolSelect(ctx, types.EmptyTSK, 0)
	for _, msg := range sms {
		found := false
		for _, selectedMsg := range selected {
			if selectedMsg.Cid() == msg.Cid() {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	ens.BeginMining(blockTime)

	kit.CircuitBreaker(t, "mine messages", mPoolThrottle, mPoolTimeout, func() bool {
		// pool pending list should be empty
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		if len(pending) == 0 {
			// all messages should be added to the chain
			for _, lookMsg := range sms {
				msgLookup, err := firstNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup)
			}
			return true
		}
		return false
	})
}

func TestMemPoolPushTwoNodes(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_CREATE_MSG_CHAINS_001, @CHAIN_MEMPOOL_SELECT_001
	//stm: @CHAIN_MEMPOOL_PENDING_001, @CHAIN_STATE_WAIT_MSG_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	//stm: @CHAIN_MEMPOOL_PUSH_002
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, secondNode, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address
	sender2 := secondNode.DefaultKey.Address

	addr, _ := firstNode.WalletNew(ctx, types.KTBLS)
	addr2, _ := secondNode.WalletNew(ctx, types.KTBLS)

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)

	const totalMessages = 10

	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	var sms []*types.SignedMessage
	// push messages to message pools of both nodes
	for i := 0; i < totalMessages; i++ {
		// first
		msg1 := &types.Message{
			From:  sender,
			To:    addr,
			Value: each,
		}

		sm1, err := firstNode.MpoolPushMessage(ctx, msg1, nil)
		require.NoError(t, err)
		require.EqualValues(t, i, sm1.Message.Nonce)
		sms = append(sms, sm1)

		// second
		msg2 := &types.Message{
			From:  sender2,
			To:    addr2,
			Value: each,
		}

		sm2, err := secondNode.MpoolPushMessage(ctx, msg2, nil)
		require.NoError(t, err)
		require.EqualValues(t, i, sm2.Message.Nonce)
		sms = append(sms, sm2)
	}

	ens.BeginMining(blockTime)

	kit.CircuitBreaker(t, "push & mine messages", mPoolThrottle, mPoolTimeout, func() bool {
		pending1, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		pending2, err := secondNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		if len(pending1) == 0 && len(pending2) == 0 {
			// Check messages on both nodes
			for _, lookMsg := range sms {
				msgLookup1, err := firstNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup1)

				msgLookup2, err := secondNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup2)
			}
			return true
		}
		return false
	})
}

func TestMemPoolClearPending(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_PUSH_001, @CHAIN_MEMPOOL_PENDING_001
	//stm: @CHAIN_STATE_WAIT_MSG_001, @CHAIN_MEMPOOL_CLEAR_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, _, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address

	addr, _ := firstNode.WalletNew(ctx, types.KTBLS)

	const totalMessages = 10

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)
	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	// Add single message, then clear the pool
	msg := &types.Message{
		From:  sender,
		To:    addr,
		Value: each,
	}
	_, err = firstNode.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	// message should be in the mempool
	kit.CircuitBreaker(t, "push message", mPoolThrottle, mPoolTimeout, func() bool {
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		return len(pending) == 1
	})

	err = firstNode.MpoolClear(ctx, true)
	require.NoError(t, err)

	// pool should be empty now
	kit.CircuitBreaker(t, "clear mempool", mPoolThrottle, mPoolTimeout, func() bool {
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		return len(pending) == 0
	})

	// mine a couple of blocks
	ens.BeginMining(blockTime)
	time.Sleep(5 * blockTime)

	// make sure that the cleared message wasn't picked up and mined
	_, err = firstNode.StateWaitMsg(ctx, msg.Cid(), 3, api.LookbackNoLimit, true)
	require.Error(t, err)
}

func TestMemPoolBatchPush(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_CREATE_MSG_CHAINS_001, @CHAIN_MEMPOOL_SELECT_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	//stm: @CHAIN_MEMPOOL_CHECK_PENDING_MESSAGES_001, @CHAIN_MEMPOOL_SELECT_001
	//stm: @CHAIN_MEMPOOL_PENDING_001, @CHAIN_STATE_WAIT_MSG_001
	//stm: @CHAIN_MEMPOOL_BATCH_PUSH_001
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, _, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address

	addr, _ := firstNode.WalletNew(ctx, types.KTBLS)

	const totalMessages = 10

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)
	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	// add messages to be mined/published
	var sms []*types.SignedMessage
	for i := 0; i < totalMessages; i++ {
		msg := &types.Message{
			From:       sender,
			To:         addr,
			Value:      each,
			Nonce:      uint64(i),
			GasLimit:   50_000_000,
			GasFeeCap:  types.NewInt(100_000_000),
			GasPremium: types.NewInt(1),
		}

		signedMessage, err := firstNode.WalletSignMessage(ctx, sender, msg)
		require.NoError(t, err)

		sms = append(sms, signedMessage)
	}

	_, err = firstNode.MpoolBatchPush(ctx, sms)
	require.NoError(t, err)

	// check pending messages for address
	kit.CircuitBreaker(t, "batch push", mPoolThrottle, mPoolTimeout, func() bool {
		msgStatuses, err := firstNode.MpoolCheckPendingMessages(ctx, sender)
		require.NoError(t, err)

		if len(msgStatuses) == totalMessages {
			for _, msgStatusList := range msgStatuses {
				for _, status := range msgStatusList {
					require.True(t, status.OK)
				}
			}
			return true
		}
		return false
	})

	// verify messages should be the ones included in the next block
	selected, _ := firstNode.MpoolSelect(ctx, types.EmptyTSK, 0)
	require.NoError(t, err)
	for _, msg := range sms {
		found := false
		for _, selectedMsg := range selected {
			if selectedMsg.Cid() == msg.Cid() {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	ens.BeginMining(blockTime)

	kit.CircuitBreaker(t, "mine messages", mPoolThrottle, mPoolTimeout, func() bool {
		// pool pending list should be empty
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		if len(pending) == 0 {
			// all messages should be added to the chain
			for _, lookMsg := range sms {
				msgLookup, err := firstNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup)
			}
			return true
		}
		return false
	})
}

func TestMemPoolPushSingleNodeUntrusted(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_CREATE_MSG_CHAINS_001, @CHAIN_MEMPOOL_SELECT_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	//stm: @CHAIN_MEMPOOL_CHECK_PENDING_MESSAGES_001, @CHAIN_MEMPOOL_SELECT_001
	//stm: @CHAIN_MEMPOOL_PENDING_001, @CHAIN_STATE_WAIT_MSG_001
	//stm: @CHAIN_MEMPOOL_PUSH_003
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, _, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address

	addr, _ := firstNode.WalletNew(ctx, types.KTBLS)

	const totalMessages = 10

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)
	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	// add messages to be mined/published
	var sms []*types.SignedMessage
	for i := 0; i < totalMessages; i++ {
		msg := &types.Message{
			From:       sender,
			To:         addr,
			Value:      each,
			Nonce:      uint64(i),
			GasLimit:   50_000_000,
			GasFeeCap:  types.NewInt(100_000_000),
			GasPremium: types.NewInt(1),
		}

		signedMessage, err := firstNode.WalletSignMessage(ctx, sender, msg)
		require.NoError(t, err)

		// push untrusted messages
		pushedCid, err := firstNode.MpoolPushUntrusted(ctx, signedMessage)
		require.NoError(t, err)
		require.Equal(t, msg.Cid(), pushedCid)

		sms = append(sms, signedMessage)
	}

	kit.CircuitBreaker(t, "push untrusted messages", mPoolThrottle, mPoolTimeout, func() bool {
		// check pending messages for address
		msgStatuses, _ := firstNode.MpoolCheckPendingMessages(ctx, sender)

		if len(msgStatuses) == totalMessages {
			for _, msgStatusList := range msgStatuses {
				for _, status := range msgStatusList {
					require.True(t, status.OK)
				}
			}
			return true
		}
		return false
	})

	// verify messages should be the ones included in the next block
	selected, _ := firstNode.MpoolSelect(ctx, types.EmptyTSK, 0)
	for _, msg := range sms {
		found := false
		for _, selectedMsg := range selected {
			if selectedMsg.Cid() == msg.Cid() {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	ens.BeginMining(blockTime)

	kit.CircuitBreaker(t, "mine untrusted messages", mPoolThrottle, mPoolTimeout, func() bool {
		// pool pending list should be empty
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		if len(pending) == 0 {
			// all messages should be added to the chain
			for _, lookMsg := range sms {
				msgLookup, err := firstNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup)
			}
			return true
		}
		return false
	})

}

func TestMemPoolBatchPushUntrusted(t *testing.T) {
	//stm: @CHAIN_MEMPOOL_CREATE_MSG_CHAINS_001, @CHAIN_MEMPOOL_SELECT_001, @CHAIN_MEMPOOL_CAP_GAS_FEE_001
	//stm: @CHAIN_MEMPOOL_CHECK_PENDING_MESSAGES_001, @CHAIN_MEMPOOL_SELECT_001
	//stm: @CHAIN_MEMPOOL_PENDING_001, @CHAIN_STATE_WAIT_MSG_001
	//stm: @CHAIN_MEMPOOL_BATCH_PUSH_002
	ctx := context.Background()
	const blockTime = 100 * time.Millisecond
	firstNode, _, _, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
	ens.InterconnectAll()
	kit.QuietMiningLogs()

	sender := firstNode.DefaultKey.Address

	addr, _ := firstNode.WalletNew(ctx, types.KTBLS)

	const totalMessages = 10

	bal, err := firstNode.WalletBalance(ctx, sender)
	require.NoError(t, err)
	toSend := big.Div(bal, big.NewInt(10))
	each := big.Div(toSend, big.NewInt(totalMessages))

	// add messages to be mined/published
	var sms []*types.SignedMessage
	for i := 0; i < totalMessages; i++ {
		msg := &types.Message{
			From:       sender,
			To:         addr,
			Value:      each,
			Nonce:      uint64(i),
			GasLimit:   50_000_000,
			GasFeeCap:  types.NewInt(100_000_000),
			GasPremium: types.NewInt(1),
		}

		signedMessage, err := firstNode.WalletSignMessage(ctx, sender, msg)
		require.NoError(t, err)

		sms = append(sms, signedMessage)
	}

	_, err = firstNode.MpoolBatchPushUntrusted(ctx, sms)
	require.NoError(t, err)

	// check pending messages for address, wait until they are all pushed
	kit.CircuitBreaker(t, "push untrusted messages", mPoolThrottle, mPoolTimeout, func() bool {
		msgStatuses, err := firstNode.MpoolCheckPendingMessages(ctx, sender)
		require.NoError(t, err)

		if len(msgStatuses) == totalMessages {
			for _, msgStatusList := range msgStatuses {
				for _, status := range msgStatusList {
					require.True(t, status.OK)
				}
			}
			return true
		}
		return false
	})

	// verify messages should be the ones included in the next block
	selected, _ := firstNode.MpoolSelect(ctx, types.EmptyTSK, 0)

	for _, msg := range sms {
		found := false
		for _, selectedMsg := range selected {
			if selectedMsg.Cid() == msg.Cid() {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	ens.BeginMining(blockTime)

	// wait until pending messages are mined, pool pending list should be empty
	kit.CircuitBreaker(t, "mine untrusted messages", mPoolThrottle, mPoolTimeout, func() bool {
		pending, err := firstNode.MpoolPending(context.TODO(), types.EmptyTSK)
		require.NoError(t, err)

		if len(pending) == 0 {
			// all messages should be added to the chain
			for _, lookMsg := range sms {
				msgLookup, err := firstNode.StateWaitMsg(ctx, lookMsg.Cid(), 3, api.LookbackNoLimit, true)
				require.NoError(t, err)
				require.NotNil(t, msgLookup)
			}
			return true
		}
		return false
	})

}
