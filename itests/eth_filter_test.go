package itests

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/bits"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	res "github.com/filecoin-project/lotus/lib/result"
)

func TestEthNewPendingTransactionFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	kit.QuietAllLogsExcept("events", "messagepool")

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// install filter
	filterID, err := client.EthNewPendingTransactionFilter(ctx)
	require.NoError(t, err)

	const iterations = 100

	// we'll send half our balance (saving the other half for gas),
	// in `iterations` increments.
	toSend := big.Div(bal, big.NewInt(2))
	each := big.Div(toSend, big.NewInt(iterations))

	waitAllCh := make(chan struct{})
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(t, err)
		<-headChangeCh // skip hccurrent

		defer func() {
			close(waitAllCh)
		}()

		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply {
						msgs, err := client.ChainGetMessagesInTipset(ctx, change.Val.Key())
						require.NoError(t, err)
						count += len(msgs)
						if count == iterations {
							return
						}
					}
				}
			}
		}
	}()

	var sms []*types.SignedMessage
	for i := 0; i < iterations; i++ {
		msg := &types.Message{
			From:  client.DefaultKey.Address,
			To:    addr,
			Value: each,
		}

		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)
		require.EqualValues(t, i, sm.Message.Nonce)

		sms = append(sms, sm)
	}

	select {
	case <-waitAllCh:
	case <-ctx.Done():
		t.Errorf("timeout waiting to pack messages")
	}

	expected := make(map[string]bool)
	for _, sm := range sms {
		hash, err := ethtypes.EthHashFromCid(sm.Cid())
		require.NoError(t, err)
		expected[hash.String()] = false
	}

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(t, err)

	// expect to have seen iteration number of mpool messages
	require.Equal(t, iterations, len(res.Results), "expected %d tipsets to have been executed", iterations)

	require.Equal(t, len(res.Results), len(expected), "expected number of filter results to equal number of messages")

	for _, txid := range res.Results {
		expected[txid.(string)] = true
	}

	for _, found := range expected {
		require.True(t, found)
	}
}

func TestEthNewPendingTransactionSub(t *testing.T) {
	require := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	kit.QuietAllLogsExcept("events", "messagepool")

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(err)

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(err)

	// install filter
	subId, err := client.EthSubscribe(ctx, res.Wrap[jsonrpc.RawParams](json.Marshal(ethtypes.EthSubscribeParams{EventType: "newPendingTransactions"})).Assert(require.NoError))
	require.NoError(err)

	var subResponses []ethtypes.EthSubscriptionResponse
	err = client.EthSubRouter.AddSub(ctx, subId, func(ctx context.Context, resp *ethtypes.EthSubscriptionResponse) error {
		subResponses = append(subResponses, *resp)
		return nil
	})
	require.NoError(err)

	const iterations = 100

	// we'll send half our balance (saving the other half for gas),
	// in `iterations` increments.
	toSend := big.Div(bal, big.NewInt(2))
	each := big.Div(toSend, big.NewInt(iterations))

	waitAllCh := make(chan struct{})
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(err)
		<-headChangeCh // skip hccurrent

		defer func() {
			close(waitAllCh)
		}()

		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply {
						msgs, err := client.ChainGetMessagesInTipset(ctx, change.Val.Key())
						require.NoError(err)
						count += len(msgs)
						if count == iterations {
							return
						}
					}
				}
			}
		}
	}()

	var sms []*types.SignedMessage
	for i := 0; i < iterations; i++ {
		msg := &types.Message{
			From:  client.DefaultKey.Address,
			To:    addr,
			Value: each,
		}

		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(err)
		require.EqualValues(i, sm.Message.Nonce)

		sms = append(sms, sm)
	}

	select {
	case <-waitAllCh:
	case <-ctx.Done():
		t.Errorf("timeout waiting to pack messages")
	}

	expected := make(map[string]bool)
	for _, sm := range sms {
		hash, err := ethtypes.EthHashFromCid(sm.Cid())
		require.NoError(err)
		expected[hash.String()] = false
	}

	// expect to have seen iteration number of mpool messages
	require.Equal(len(subResponses), len(expected), "expected number of filter results to equal number of messages")

	for _, txid := range subResponses {
		expected[txid.Result.(string)] = true
	}

	for _, found := range expected {
		require.True(found)
	}
}

func TestEthNewBlockFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	kit.QuietAllLogsExcept("events", "messagepool")

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// install filter
	filterID, err := client.EthNewBlockFilter(ctx)
	require.NoError(t, err)

	const iterations = 30

	// we'll send half our balance (saving the other half for gas),
	// in `iterations` increments.
	toSend := big.Div(bal, big.NewInt(2))
	each := big.Div(toSend, big.NewInt(iterations))

	waitAllCh := make(chan struct{})
	tipsetChan := make(chan *types.TipSet, iterations)
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(t, err)
		<-headChangeCh // skip hccurrent

		defer func() {
			close(tipsetChan)
			close(waitAllCh)
		}()

		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply || change.Type == store.HCRevert {
						count++
						tipsetChan <- change.Val
						if count == iterations {
							return
						}
					}
				}
			}
		}
	}()

	for i := 0; i < iterations; i++ {
		msg := &types.Message{
			From:  client.DefaultKey.Address,
			To:    addr,
			Value: each,
		}

		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)
		require.EqualValues(t, i, sm.Message.Nonce)
	}

	select {
	case <-waitAllCh:
	case <-ctx.Done():
		t.Errorf("timeout waiting to pack messages")
	}

	expected := make(map[string]bool)
	for ts := range tipsetChan {
		c, err := ts.Key().Cid()
		require.NoError(t, err)
		hash, err := ethtypes.EthHashFromCid(c)
		require.NoError(t, err)
		expected[hash.String()] = false
	}

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(t, err)

	// expect to have seen iteration number of tipsets
	require.Equal(t, iterations, len(res.Results), "expected %d tipsets to have been executed", iterations)

	require.Equal(t, len(res.Results), len(expected), "expected number of filter results to equal number of tipsets")

	for _, blockhash := range res.Results {
		expected[blockhash.(string)] = true
	}

	for _, found := range expected {
		require.True(t, found, "expected all tipsets to be present in filter results")
	}
}

func TestEthNewFilterDefaultSpec(t *testing.T) {
	require := require.New(t)

	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/events.bin")
	require.NoError(err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(err)

	result := client.EVM().DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(err)
	t.Logf("actor ID address is %s", idAddr)

	// install filter
	filterID, err := client.EthNewFilter(ctx, &ethtypes.EthFilterSpec{})
	require.NoError(err)

	const iterations = 3
	ethContractAddr, received := invokeLogFourData(t, client, iterations)

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(err)

	// expect to have seen iteration number of events
	require.Equal(iterations, len(res.Results))

	expected := []ExpectedEthLog{
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				paddedEthHash([]byte{0x11, 0x11}),
				paddedEthHash([]byte{0x22, 0x22}),
				paddedEthHash([]byte{0x33, 0x33}),
				paddedEthHash([]byte{0x44, 0x44}),
			},
			Data: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		},
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				paddedEthHash([]byte{0x11, 0x11}),
				paddedEthHash([]byte{0x22, 0x22}),
				paddedEthHash([]byte{0x33, 0x33}),
				paddedEthHash([]byte{0x44, 0x44}),
			},
			Data: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		},
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				paddedEthHash([]byte{0x11, 0x11}),
				paddedEthHash([]byte{0x22, 0x22}),
				paddedEthHash([]byte{0x33, 0x33}),
				paddedEthHash([]byte{0x44, 0x44}),
			},
			Data: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		},
	}

	elogs, err := parseEthLogsFromFilterResult(res)
	require.NoError(err)
	AssertEthLogs(t, elogs, expected, received)
}

func TestEthGetLogsBasic(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	invocations := 1
	ethContractAddr, received := invokeLogFourData(t, client, invocations)

	// Build filter spec
	spec := kit.NewEthFilterBuilder().
		FromBlockEpoch(0).
		Topic1OneOf(paddedEthHash([]byte{0x11, 0x11})).
		Filter()

	expected := []ExpectedEthLog{
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				paddedEthHash([]byte{0x11, 0x11}),
				paddedEthHash([]byte{0x22, 0x22}),
				paddedEthHash([]byte{0x33, 0x33}),
				paddedEthHash([]byte{0x44, 0x44}),
			},
			Data: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		},
	}

	// Use filter
	res, err := client.EthGetLogs(ctx, spec)
	require.NoError(err)

	elogs, err := parseEthLogsFromFilterResult(res)
	require.NoError(err)
	AssertEthLogs(t, elogs, expected, received)
}

func TestEthSubscribeLogsNoTopicSpec(t *testing.T) {
	require := require.New(t)

	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/events.bin")
	require.NoError(err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(err)

	result := client.EVM().DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(err)
	t.Logf("actor ID address is %s", idAddr)

	// install filter
	subId, err := client.EthSubscribe(ctx, res.Wrap[jsonrpc.RawParams](json.Marshal(ethtypes.EthSubscribeParams{EventType: "logs"})).Assert(require.NoError))
	require.NoError(err)

	var subResponses []ethtypes.EthSubscriptionResponse
	err = client.EthSubRouter.AddSub(ctx, subId, func(ctx context.Context, resp *ethtypes.EthSubscriptionResponse) error {
		subResponses = append(subResponses, *resp)
		return nil
	})
	require.NoError(err)

	const iterations = 10
	ethContractAddr, messages := invokeLogFourData(t, client, iterations)

	expected := make([]ExpectedEthLog, iterations)
	for i := range expected {
		expected[i] = ExpectedEthLog{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				paddedEthHash([]byte{0x11, 0x11}),
				paddedEthHash([]byte{0x22, 0x22}),
				paddedEthHash([]byte{0x33, 0x33}),
				paddedEthHash([]byte{0x44, 0x44}),
			},
			Data: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		}
	}

	elogs, err := parseEthLogsFromSubscriptionResponses(subResponses)
	require.NoError(err)
	AssertEthLogs(t, elogs, expected, messages)
}

func TestTxReceiptBloom(t *testing.T) {
	blockTime := 50 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/EventMatrix.hex")

	_, ml, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "logEventZeroData()", nil)
	require.NoError(t, err)

	th, err := client.EthGetTransactionHashByCid(ctx, ml.Message)
	require.NoError(t, err)
	require.NotNil(t, th)

	receipt, err := client.EthGetTransactionReceipt(ctx, *th)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Len(t, receipt.Logs, 1)

	// computed by calling EventMatrix/logEventZeroData in remix
	// note this only contains topic bits
	matchMask := "0x00000000000000000000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	maskBytes, err := hex.DecodeString(matchMask[2:])
	require.NoError(t, err)

	bitsSet := 0
	for i, maskByte := range maskBytes {
		bitsSet += bits.OnesCount8(receipt.LogsBloom[i])

		if maskByte > 0 {
			require.True(t, maskByte&receipt.LogsBloom[i] > 0)
		}
	}

	// 3 bits from the topic, 3 bits from the address
	require.Equal(t, 6, bitsSet)
}

func TestEthGetLogs(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	contract1, contract2, invocations := prepareEventMatrixInvocations(ctx, t, client)
	testCases := getCombinationFilterTestCases(contract1, contract2, "0x0")

	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {
			res, err := client.EthGetLogs(ctx, tc.spec)
			require.NoError(err)

			elogs, err := parseEthLogsFromFilterResult(res)
			require.NoError(err)
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}

func TestEthGetFilterChanges(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	contract1, contract2, invocations := prepareEventMatrixInvocations(ctx, t, client)

	// Get the test cases
	testCases := getCombinationFilterTestCases(contract1, contract2, "latest")

	testFilters := map[string]ethtypes.EthFilterID{}
	// Create all the filters
	for _, tc := range testCases {
		filterID, err := client.EthNewFilter(ctx, tc.spec)
		require.NoError(err)
		testFilters[tc.name] = filterID
	}

	// Perform all the invocations
	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {
			filterID, ok := testFilters[tc.name]
			require.True(ok)

			// Look for events that the filter has accumulated
			res, err := client.EthGetFilterChanges(ctx, filterID)
			require.NoError(err)

			elogs, err := parseEthLogsFromFilterResult(res)
			require.NoError(err)
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}

func TestEthSubscribeLogs(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	contract1, contract2, invocations := prepareEventMatrixInvocations(ctx, t, client)

	// Get the test cases
	testCases := getTopicFilterTestCases(contract1, contract2, "latest")

	testResponses := map[string]chan ethtypes.EthSubscriptionResponse{}

	// Create all the filters
	for _, tc := range testCases {
		// subscribe to topics in filter
		subParam, err := json.Marshal(ethtypes.EthSubscribeParams{
			EventType: "logs",
			Params: &ethtypes.EthSubscriptionParams{
				Topics:  tc.spec.Topics,
				Address: tc.spec.Address,
			},
		})
		require.NoError(err)

		subId, err := client.EthSubscribe(ctx, subParam)
		require.NoError(err)

		responseCh := make(chan ethtypes.EthSubscriptionResponse, len(invocations))
		testResponses[tc.name] = responseCh

		err = client.EthSubRouter.AddSub(ctx, subId, func(ctx context.Context, resp *ethtypes.EthSubscriptionResponse) error {
			responseCh <- *resp
			return nil
		})
		require.NoError(err)
	}

	// Perform all the invocations
	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	// wait a little for subscriptions to gather results
	time.Sleep(blockTime * 6)

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {
			responseCh, ok := testResponses[tc.name]
			require.True(ok)

			// don't expect any more responses
			close(responseCh)

			var elogs []*ethtypes.EthLog
			for resp := range responseCh {
				rmap, ok := resp.Result.(map[string]interface{})
				require.True(ok, "expected subscription result entry to be map[string]interface{}, but was %T", resp.Result)

				elog, err := ParseEthLog(rmap)
				require.NoError(err)

				elogs = append(elogs, elog)
			}
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}

func TestEthGetFilterLogs(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	contract1, contract2, invocations := prepareEventMatrixInvocations(ctx, t, client)

	// Get the test cases
	testCases := getCombinationFilterTestCases(contract1, contract2, "latest")

	testFilters := map[string]ethtypes.EthFilterID{}
	// Create all the filters
	for _, tc := range testCases {
		filterID, err := client.EthNewFilter(ctx, tc.spec)
		require.NoError(err)
		testFilters[tc.name] = filterID
	}

	// Perform all the invocations
	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {
			filterID, ok := testFilters[tc.name]
			require.True(ok)

			// Look for events that the filter has accumulated
			res, err := client.EthGetFilterLogs(ctx, filterID)
			require.NoError(err)

			elogs, err := parseEthLogsFromFilterResult(res)
			require.NoError(err)
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}

func TestEthGetLogsWithBlockRanges(t *testing.T) {
	require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	_, _, messages := invokeEventMatrix(ctx, t, client)

	// Organize expected logs into three partitions for range testing
	expectedByHeight := map[abi.ChainEpoch][]ExpectedEthLog{}
	distinctHeights := map[abi.ChainEpoch]bool{}

	// Select events for partitioning
	for _, m := range messages {
		if bytes.Equal(m.invocation.Selector, kit.EventMatrixContract.Fn["logEventTwoIndexedWithData"]) {
			addr := getEthAddress(ctx, t, client, m.invocation.Target)
			args := unpackUint64Values(m.invocation.Data)
			require.Equal(3, len(args), "logEventTwoIndexedWithData should have 3 arguments")

			distinctHeights[m.ts.Height()] = true
			expectedByHeight[m.ts.Height()] = append(expectedByHeight[m.ts.Height()], ExpectedEthLog{
				Address: addr,
				Topics: []ethtypes.EthHash{
					kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
					uint64EthHash(args[0]),
					uint64EthHash(args[1]),
				},
				Data: paddedUint64(args[2]),
			})
		}
	}

	// Divide heights into 3 partitions, they don't have to be equal
	require.True(len(distinctHeights) >= 3, "expected slice should divisible into three partitions")
	heights := make([]abi.ChainEpoch, 0, len(distinctHeights))
	for h := range distinctHeights {
		heights = append(heights, h)
	}
	sort.Slice(heights, func(i, j int) bool {
		return heights[i] < heights[j]
	})
	heightsPerPartition := len(heights) / 3

	type partition struct {
		start    abi.ChainEpoch
		end      abi.ChainEpoch
		expected []ExpectedEthLog
	}

	var partition1, partition2, partition3 partition

	partition1.start = heights[0]
	partition1.end = heights[heightsPerPartition-1]
	for e := partition1.start; e <= partition1.end; e++ {
		exp, ok := expectedByHeight[e]
		if !ok {
			continue
		}
		partition1.expected = append(partition1.expected, exp...)
	}
	t.Logf("partition1 from %d to %d with %d expected", partition1.start, partition1.end, len(partition1.expected))
	require.True(len(partition1.expected) > 0, "partition should have events")

	partition2.start = heights[heightsPerPartition]
	partition2.end = heights[heightsPerPartition*2-1]
	for e := partition2.start; e <= partition2.end; e++ {
		exp, ok := expectedByHeight[e]
		if !ok {
			continue
		}
		partition2.expected = append(partition2.expected, exp...)
	}
	t.Logf("partition2 from %d to %d with %d expected", partition2.start, partition2.end, len(partition2.expected))
	require.True(len(partition2.expected) > 0, "partition should have events")

	partition3.start = heights[heightsPerPartition*2]
	partition3.end = heights[len(heights)-1]
	for e := partition3.start; e <= partition3.end; e++ {
		exp, ok := expectedByHeight[e]
		if !ok {
			continue
		}
		partition3.expected = append(partition3.expected, exp...)
	}
	t.Logf("partition3 from %d to %d with %d expected", partition3.start, partition3.end, len(partition3.expected))
	require.True(len(partition3.expected) > 0, "partition should have events")

	// these are the topics we selected for partitioning earlier
	topics := []ethtypes.EthHash{kit.EventMatrixContract.Ev["EventTwoIndexedWithData"]}

	union := func(lists ...[]ExpectedEthLog) []ExpectedEthLog {
		ret := []ExpectedEthLog{}
		for _, list := range lists {
			ret = append(ret, list...)
		}
		return ret
	}

	testCases := []struct {
		name     string
		spec     *ethtypes.EthFilterSpec
		expected []ExpectedEthLog
	}{
		{
			name:     "find all events from genesis",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(0).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected, partition2.expected, partition3.expected),
		},

		{
			name:     "find all from start of partition1",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition1.start).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected, partition2.expected, partition3.expected),
		},

		{
			name:     "find all from start of partition2",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition2.start).Topic1OneOf(topics...).Filter(),
			expected: union(partition2.expected, partition3.expected),
		},

		{
			name:     "find all from start of partition3",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition3.start).Topic1OneOf(topics...).Filter(),
			expected: union(partition3.expected),
		},

		{
			name:     "find none after end of partition3",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition3.end + 1).Topic1OneOf(topics...).Filter(),
			expected: nil,
		},

		{
			name:     "find all events from genesis to end of partition1",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(0).ToBlockEpoch(partition1.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected),
		},

		{
			name:     "find all events from genesis to end of partition2",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(0).ToBlockEpoch(partition2.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected, partition2.expected),
		},

		{
			name:     "find all events from genesis to end of partition3",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(0).ToBlockEpoch(partition3.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected, partition2.expected, partition3.expected),
		},

		{
			name:     "find none from genesis to start of partition1",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(0).ToBlockEpoch(partition1.start - 1).Topic1OneOf(topics...).Filter(),
			expected: nil,
		},

		{
			name:     "find all events in partition1",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition1.start).ToBlockEpoch(partition1.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected),
		},

		{
			name:     "find all events in partition2",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition2.start).ToBlockEpoch(partition2.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition2.expected),
		},

		{
			name:     "find all events in partition3",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition3.start).ToBlockEpoch(partition3.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition3.expected),
		},

		{
			name:     "find all events from earliest to end of partition1",
			spec:     kit.NewEthFilterBuilder().FromBlock("earliest").ToBlockEpoch(partition1.end).Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected),
		},

		{
			name:     "find all events from start of partition3 to latest",
			spec:     kit.NewEthFilterBuilder().FromBlockEpoch(partition3.start).ToBlock("latest").Topic1OneOf(topics...).Filter(),
			expected: union(partition3.expected),
		},

		{
			name:     "find all events from earliest to latest",
			spec:     kit.NewEthFilterBuilder().FromBlock("earliest").ToBlock("latest").Topic1OneOf(topics...).Filter(),
			expected: union(partition1.expected, partition2.expected, partition3.expected),
		},
	}

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {
			res, err := client.EthGetLogs(ctx, tc.spec)
			require.NoError(err)

			elogs, err := parseEthLogsFromFilterResult(res)
			require.NoError(err)
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}

func TestEthNewFilterMergesHistoricWithRealtime(t *testing.T) {
	require := require.New(t)

	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	sender, contract := client.EVM().DeployContractFromFilename(ctx, kit.EventMatrixContract.Filename)

	// generate some events before the creation of the filter
	preInvocations := []Invocation{
		{
			Sender:   sender,
			Target:   contract,
			Selector: kit.EventMatrixContract.Fn["logEventOneData"],
			Data:     packUint64Values(1),
		},
		{
			Sender:   sender,
			Target:   contract,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexed"],
			Data:     packUint64Values(2),
		},
	}

	messages := invokeAndWaitUntilAllOnChain(t, client, preInvocations)

	// now install filter
	spec := kit.NewEthFilterBuilder().FromBlock("earliest").Filter()

	filterID, err := client.EthNewFilter(ctx, spec)
	require.NoError(err)

	// generate some events after the creation of the filter
	postInvocations := []Invocation{
		{
			Sender:   sender,
			Target:   contract,
			Selector: kit.EventMatrixContract.Fn["logEventOneData"],
			Data:     packUint64Values(3),
		},
		{
			Sender:   sender,
			Target:   contract,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexed"],
			Data:     packUint64Values(4),
		},
	}

	postMessages := invokeAndWaitUntilAllOnChain(t, client, postInvocations)
	for k, v := range postMessages {
		messages[k] = v
	}

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(err)

	ethContractAddr := getEthAddress(ctx, t, client, contract)

	// expect to see 2 messages from before the filter was installed and 2 after
	expected := []ExpectedEthLog{
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				kit.EventMatrixContract.Ev["EventOneData"],
			},
			Data: paddedUint64(1),
		},
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				kit.EventMatrixContract.Ev["EventOneIndexed"],
				uint64EthHash(2),
			},
		},
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				kit.EventMatrixContract.Ev["EventOneData"],
			},
			Data: paddedUint64(3),
		},
		{
			Address: ethContractAddr,
			Topics: []ethtypes.EthHash{
				kit.EventMatrixContract.Ev["EventOneIndexed"],
				uint64EthHash(4),
			},
		},
	}

	elogs, err := parseEthLogsFromFilterResult(res)
	require.NoError(err)
	AssertEthLogs(t, elogs, expected, messages)
}

// -------------------------------------------------------------------------------
// end of tests
// -------------------------------------------------------------------------------

type msgInTipset struct {
	invocation Invocation // the solidity invocation that generated this message
	msg        api.Message
	events     []types.Event // events extracted from receipt
	ts         *types.TipSet
	reverted   bool
}

func getEthAddress(ctx context.Context, t *testing.T, client *kit.TestFullNode, addr address.Address) ethtypes.EthAddress {
	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	actor, err := client.StateGetActor(ctx, addr, head.Key())
	require.NoError(t, err)
	require.NotNil(t, actor.Address)
	ethContractAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.Address)
	require.NoError(t, err)
	return ethContractAddr
}

type Invocation struct {
	Sender    address.Address
	Target    address.Address
	Selector  []byte // function selector
	Data      []byte
	MinHeight abi.ChainEpoch // minimum chain height that must be reached before invoking
}

func invokeAndWaitUntilAllOnChain(t *testing.T, client *kit.TestFullNode, invocations []Invocation) map[ethtypes.EthHash]msgInTipset {
	require := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	msgChan := make(chan msgInTipset, len(invocations))

	waitAllCh := make(chan struct{})
	waitForFirstHeadChange := make(chan struct{})
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(err)
		select {
		case <-ctx.Done():
			return
		case <-headChangeCh: // skip hccurrent
		}

		close(waitForFirstHeadChange)

		defer func() {
			close(msgChan)
			close(waitAllCh)
		}()

		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply || change.Type == store.HCRevert {
						msgs, err := client.ChainGetMessagesInTipset(ctx, change.Val.Key())
						require.NoError(err)

						count += len(msgs)
						for _, m := range msgs {
							select {
							case msgChan <- msgInTipset{msg: m, ts: change.Val, reverted: change.Type == store.HCRevert}:
							default:
							}
						}

						if count == len(invocations) {
							return
						}
					}
				}
			}
		}
	}()

	select {
	case <-waitForFirstHeadChange:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for first head change")
	}

	eventMap := map[cid.Cid][]types.Event{}
	invocationMap := map[cid.Cid]Invocation{}
	for _, inv := range invocations {
		if inv.MinHeight > 0 {
			for {
				ts, err := client.ChainHead(ctx)
				require.NoError(err)
				if ts.Height() >= inv.MinHeight {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("context cancelled")
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		ret, err := client.EVM().InvokeSolidity(ctx, inv.Sender, inv.Target, inv.Selector, inv.Data)
		require.NoError(err)
		require.True(ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")

		invocationMap[ret.Message] = inv

		require.NotNil(t, ret.Receipt.EventsRoot, "no event root on receipt")

		evs := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
		eventMap[ret.Message] = evs
	}

	select {
	case <-waitAllCh:
	case <-ctx.Done():
		t.Fatalf("timeout waiting to pack messages")
	}

	received := make(map[ethtypes.EthHash]msgInTipset)
	for m := range msgChan {
		inv, ok := invocationMap[m.msg.Cid]
		require.True(ok)
		m.invocation = inv

		evs, ok := eventMap[m.msg.Cid]
		require.True(ok)
		m.events = evs

		eh, err := client.EthGetTransactionHashByCid(ctx, m.msg.Cid)
		require.NoError(err)
		received[*eh] = m
	}
	require.Equal(len(invocations), len(received), "all messages on chain")

	return received
}

func invokeLogFourData(t *testing.T, client *kit.TestFullNode, iterations int) (ethtypes.EthAddress, map[ethtypes.EthHash]msgInTipset) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, kit.EventsContract.Filename)

	invocations := make([]Invocation, iterations)
	for i := range invocations {
		invocations[i] = Invocation{
			Sender:   fromAddr,
			Target:   idAddr,
			Selector: kit.EventsContract.Fn["log_four_data"],
			Data:     nil,
		}
	}

	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	ethAddr := getEthAddress(ctx, t, client, idAddr)

	return ethAddr, messages
}

func prepareEventMatrixInvocations(ctx context.Context, t *testing.T, client *kit.TestFullNode) (ethtypes.EthAddress, ethtypes.EthAddress, []Invocation) {
	sender1, contract1 := client.EVM().DeployContractFromFilename(ctx, kit.EventMatrixContract.Filename)
	sender2, contract2 := client.EVM().DeployContractFromFilename(ctx, kit.EventMatrixContract.Filename)

	invocations := []Invocation{
		// log EventZeroData()
		// topic1: hash(EventZeroData)
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventZeroData"],
			Data:     nil,
		},

		// log EventOneData(23)
		// topic1: hash(EventOneData)
		// data: 23
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventOneData"],
			Data:     packUint64Values(23),
		},

		// log EventOneIndexed(44)
		// topic1: hash(EventOneIndexed)
		// topic2: 44
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexed"],
			Data:     packUint64Values(44),
		},

		// log EventTwoIndexed(44,19) from contract2
		// topic1: hash(EventTwoIndexed)
		// topic2: 44
		// topic3: 19
		{
			Sender:   sender2,
			Target:   contract2,
			Selector: kit.EventMatrixContract.Fn["logEventTwoIndexed"],
			Data:     packUint64Values(44, 19),
		},

		// log EventOneData(44)
		// topic1: hash(EventOneData)
		// data: 44
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventOneData"],
			Data:     packUint64Values(44),
		},

		// log EventTwoData(555,666)
		// topic1: hash(EventTwoData)
		// data: 555,666
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventTwoData"],
			Data:     packUint64Values(555, 666),
		},

		// log EventZeroData() from contract2
		// topic1: hash(EventZeroData)
		{
			Sender:   sender2,
			Target:   contract2,
			Selector: kit.EventMatrixContract.Fn["logEventZeroData"],
			Data:     nil,
		},

		// log EventThreeData(1,2,3)
		// topic1: hash(EventTwoData)
		// data: 1,2,3
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventThreeData"],
			Data:     packUint64Values(1, 2, 3),
		},

		// log EventThreeIndexed(44,27,19) from contract2
		// topic1: hash(EventThreeIndexed)
		// topic2: 44
		// topic3: 27
		// topic4: 19
		{
			Sender:   sender1,
			Target:   contract2,
			Selector: kit.EventMatrixContract.Fn["logEventThreeIndexed"],
			Data:     packUint64Values(44, 27, 19),
		},

		// log EventOneIndexedWithData(44,19)
		// topic1: hash(EventOneIndexedWithData)
		// topic2: 44
		// data: 19
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexedWithData"],
			Data:     packUint64Values(44, 19),
		},

		// log EventOneIndexedWithData(46,12)
		// topic1: hash(EventOneIndexedWithData)
		// topic2: 46
		// data: 12
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexedWithData"],
			Data:     packUint64Values(46, 12),
		},

		// log EventTwoIndexedWithData(44,27,19)
		// topic1: hash(EventTwoIndexedWithData)
		// topic2: 44
		// topic3: 27
		// data: 19
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventTwoIndexedWithData"],
			Data:     packUint64Values(44, 27, 19),
		},

		// log EventThreeIndexedWithData(44,27,19,12)
		// topic1: hash(EventThreeIndexedWithData)
		// topic2: 44
		// topic3: 27
		// topic4: 19
		// data: 12
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventThreeIndexedWithData"],
			Data:     packUint64Values(44, 27, 19, 12),
		},

		// log EventOneIndexedWithData(50,9)
		// topic1: hash(EventOneIndexedWithData)
		// topic2: 50
		// data: 9
		{
			Sender:   sender2,
			Target:   contract2,
			Selector: kit.EventMatrixContract.Fn["logEventOneIndexedWithData"],
			Data:     packUint64Values(50, 9),
		},

		// log EventTwoIndexedWithData(46,27,19)
		// topic1: hash(EventTwoIndexedWithData)
		// topic2: 46
		// topic3: 27
		// data: 19
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventTwoIndexedWithData"],
			Data:     packUint64Values(46, 27, 19),
		},

		// log EventTwoIndexedWithData(46,14,19)
		// topic1: hash(EventTwoIndexedWithData)
		// topic2: 46
		// topic3: 14
		// data: 19
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventTwoIndexedWithData"],
			Data:     packUint64Values(46, 14, 19),
		},
		// log EventTwoIndexed(44,19) from contract1
		// topic1: hash(EventTwoIndexed)
		// topic2: 44
		// topic3: 19
		{
			Sender:   sender1,
			Target:   contract1,
			Selector: kit.EventMatrixContract.Fn["logEventTwoIndexed"],
			Data:     packUint64Values(40, 20),
		},
	}

	ethAddr1 := getEthAddress(ctx, t, client, contract1)
	ethAddr2 := getEthAddress(ctx, t, client, contract2)

	return ethAddr1, ethAddr2, invocations
}

func invokeEventMatrix(ctx context.Context, t *testing.T, client *kit.TestFullNode) (ethtypes.EthAddress, ethtypes.EthAddress, map[ethtypes.EthHash]msgInTipset) {
	ethAddr1, ethAddr2, invocations := prepareEventMatrixInvocations(ctx, t, client)
	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)
	return ethAddr1, ethAddr2, messages
}

type filterTestCase struct {
	name     string
	spec     *ethtypes.EthFilterSpec
	expected []ExpectedEthLog
}

// getTopicFilterTestCases returns filter test cases that only include topic criteria
func getTopicFilterTestCases(contract1, contract2 ethtypes.EthAddress, fromBlock string) []filterTestCase {
	return []filterTestCase{
		{
			name: "find all EventZeroData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventZeroData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventZeroData"],
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventZeroData"],
					},
					Data: nil,
				},
			},
		},
		{
			name: "find all EventOneData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventOneData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneData"],
					},
					Data: packUint64Values(23),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneData"],
					},
					Data: packUint64Values(44),
				},
			},
		},
		{
			name: "find all EventTwoData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventTwoData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoData"],
					},
					Data: packUint64Values(555, 666),
				},
			},
		},
		{
			name: "find all EventThreeData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventThreeData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeData"],
					},
					Data: packUint64Values(1, 2, 3),
				},
			},
		},
		{
			name: "find all EventOneIndexed events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventOneIndexed"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexed"],
						uint64EthHash(44),
					},
					Data: nil,
				},
			},
		},
		{
			name: "find all EventTwoIndexed events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventTwoIndexed"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexed"],
						uint64EthHash(44),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexed"],
						uint64EthHash(40),
						uint64EthHash(20),
					},
					Data: nil,
				},
			},
		},
		{
			name: "find all EventThreeIndexed events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventThreeIndexed"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexed"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: nil,
				},
			},
		},
		{
			name: "find all EventOneIndexedWithData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventOneIndexedWithData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(44),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(46),
					},
					Data: paddedUint64(12),
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(50),
					},
					Data: paddedUint64(9),
				},
			},
		},
		{
			name: "find all EventTwoIndexedWithData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventTwoIndexedWithData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(14),
					},
					Data: paddedUint64(19),
				},
			},
		},
		{
			name: "find all EventThreeIndexedWithData events",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic1OneOf(kit.EventMatrixContract.Ev["EventThreeIndexedWithData"]).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: paddedUint64(12),
				},
			},
		},

		{
			name: "find all events with topic2 of 44",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic2OneOf(uint64EthHash(44)).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexed"],
						uint64EthHash(44),
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexed"],
						uint64EthHash(44),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexed"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(44),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: paddedUint64(12),
				},
			},
		},
		{
			name: "find all events with topic2 of 46",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic2OneOf(uint64EthHash(46)).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(46),
					},
					Data: paddedUint64(12),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(14),
					},
					Data: paddedUint64(19),
				},
			},
		},
		{
			name: "find all events with topic2 of 50",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic2OneOf(uint64EthHash(50)).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(50),
					},
					Data: paddedUint64(9),
				},
			},
		},
		{
			name: "find all events with topic2 of 46 or 50",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).Topic2OneOf(uint64EthHash(46), uint64EthHash(50)).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(46),
					},
					Data: paddedUint64(12),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(14),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(50),
					},
					Data: paddedUint64(9),
				},
			},
		},

		{
			name: "find all events with topic1 of EventTwoIndexedWithData and topic3 of 27",
			spec: kit.NewEthFilterBuilder().
				FromBlockEpoch(0).
				Topic1OneOf(kit.EventMatrixContract.Ev["EventTwoIndexedWithData"]).
				Topic3OneOf(uint64EthHash(27)).
				Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(46),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
			},
		},

		{
			name: "find all events with topic1 of EventTwoIndexedWithData or EventOneIndexed and topic2 of 44",
			spec: kit.NewEthFilterBuilder().
				FromBlockEpoch(0).
				Topic1OneOf((kit.EventMatrixContract.Ev["EventTwoIndexedWithData"]), kit.EventMatrixContract.Ev["EventOneIndexed"]).
				Topic2OneOf(uint64EthHash(44)).
				Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexedWithData"],
						uint64EthHash(44),
						uint64EthHash(27),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexed"],
						uint64EthHash(44),
					},
					Data: nil,
				},
			},
		},
	}
}

// getAddressFilterTestCases returns filter test cases include address criteria
func getAddressFilterTestCases(contract1, contract2 ethtypes.EthAddress, fromBlock string) []filterTestCase {
	return []filterTestCase{
		{
			name: "find all events from contract2",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).AddressOneOf(contract2).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventZeroData"],
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexed"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexed"],
						uint64EthHash(44),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(50),
					},
					Data: paddedUint64(9),
				},
			},
		},

		{
			name: "find all events with topic2 of 44 from contract2",
			spec: kit.NewEthFilterBuilder().FromBlock(fromBlock).AddressOneOf(contract2).Topic2OneOf(paddedEthHash(paddedUint64(44))).Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventThreeIndexed"],
						uint64EthHash(44),
						uint64EthHash(27),
						uint64EthHash(19),
					},
					Data: nil,
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventTwoIndexed"],
						uint64EthHash(44),
						uint64EthHash(19),
					},
					Data: nil,
				},
			},
		},

		{
			name: "find all EventOneIndexedWithData events from contract1 or contract2",
			spec: kit.NewEthFilterBuilder().
				FromBlockEpoch(0).
				AddressOneOf(contract1, contract2).
				Topic1OneOf(kit.EventMatrixContract.Ev["EventOneIndexedWithData"]).
				Filter(),

			expected: []ExpectedEthLog{
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(44),
					},
					Data: paddedUint64(19),
				},
				{
					Address: contract1,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(46),
					},
					Data: paddedUint64(12),
				},
				{
					Address: contract2,
					Topics: []ethtypes.EthHash{
						kit.EventMatrixContract.Ev["EventOneIndexedWithData"],
						uint64EthHash(50),
					},
					Data: paddedUint64(9),
				},
			},
		},
	}
}

func getCombinationFilterTestCases(contract1, contract2 ethtypes.EthAddress, fromBlock string) []filterTestCase {
	topicCases := getTopicFilterTestCases(contract1, contract2, fromBlock)
	addressCases := getAddressFilterTestCases(contract1, contract2, fromBlock)

	return append(topicCases, addressCases...)
}

type ExpectedEthLog struct {
	// Address is the address of the actor that produced the event log.
	Address ethtypes.EthAddress `json:"address"`

	// List of topics associated with the event log.
	Topics []ethtypes.EthHash `json:"topics"`

	// Data is the value of the event log, excluding topics
	Data ethtypes.EthBytes `json:"data"`
}

func AssertEthLogs(t *testing.T, actual []*ethtypes.EthLog, expected []ExpectedEthLog, messages map[ethtypes.EthHash]msgInTipset) {
	t.Helper()
	require := require.New(t)

	t.Logf("got %d ethlogs, wanted %d", len(actual), len(expected))

	formatTopics := func(topics []ethtypes.EthHash) string {
		ss := make([]string, len(topics))
		for i := range topics {
			ss[i] = fmt.Sprintf("%d:%s", i, topics[i])
		}
		return strings.Join(ss, ",")
	}

	expectedMatched := map[int]bool{}

	for _, elog := range actual {
		msg, exists := messages[elog.TransactionHash]
		require.True(exists, "message not seen on chain")

		tsCid, err := msg.ts.Key().Cid()
		require.NoError(err)

		tsCidHash, err := ethtypes.EthHashFromCid(tsCid)
		require.NoError(err)

		require.Equal(tsCidHash, elog.BlockHash, "block hash matches tipset key")

		// Try and match the received log against an expected log
		matched := false
	LoopExpected:
		for i, want := range expected {
			// each expected log must match only once
			if expectedMatched[i] {
				continue
			}

			if elog.Address != want.Address {
				continue
			}

			if len(elog.Topics) != len(want.Topics) {
				continue
			}

			for j := range elog.Topics {
				if elog.Topics[j] != want.Topics[j] {
					continue LoopExpected
				}
			}

			if !bytes.Equal(elog.Data, want.Data) {
				continue
			}

			expectedMatched[i] = true
			matched = true
			break
		}

		if !matched {
			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("found unexpected log at height %d:\n", msg.ts.Height()))
			buf.WriteString(fmt.Sprintf("  address: %s\n", elog.Address))
			buf.WriteString(fmt.Sprintf("  topics: %s\n", formatTopics(elog.Topics)))
			buf.WriteString(fmt.Sprintf("  data: %x\n", elog.Data))
			buf.WriteString("original events from receipt were:\n")
			for i, ev := range msg.events {
				buf.WriteString(fmt.Sprintf("event %d\n", i))
				buf.WriteString(fmt.Sprintf("  emitter: %v\n", ev.Emitter))
				for _, en := range ev.Entries {
					buf.WriteString(fmt.Sprintf("  %s=%x\n", en.Key, en.Value))
				}
			}

			t.Errorf(buf.String())
		}
	}

	for i := range expected {
		if _, ok := expectedMatched[i]; !ok {
			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("did not find expected log with index %d:\n", i))
			buf.WriteString(fmt.Sprintf("  address: %s\n", expected[i].Address))
			buf.WriteString(fmt.Sprintf("  topics: %s\n", formatTopics(expected[i].Topics)))
			buf.WriteString(fmt.Sprintf("  data: %x\n", expected[i].Data))
			t.Errorf(buf.String())
		}
	}
}

func parseEthLogsFromSubscriptionResponses(subResponses []ethtypes.EthSubscriptionResponse) ([]*ethtypes.EthLog, error) {
	elogs := make([]*ethtypes.EthLog, 0, len(subResponses))
	for i := range subResponses {
		rmap, ok := subResponses[i].Result.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("expected subscription result entry to be map[string]interface{}, but was %T", subResponses[i].Result)
		}

		elog, err := ParseEthLog(rmap)
		if err != nil {
			return nil, err
		}
		elogs = append(elogs, elog)
	}

	return elogs, nil
}

func parseEthLogsFromFilterResult(res *ethtypes.EthFilterResult) ([]*ethtypes.EthLog, error) {
	elogs := make([]*ethtypes.EthLog, 0, len(res.Results))

	for _, r := range res.Results {
		rmap, ok := r.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("expected filter result entry to be map[string]interface{}, but was %T", r)
		}

		elog, err := ParseEthLog(rmap)
		if err != nil {
			return nil, err
		}
		elogs = append(elogs, elog)
	}

	return elogs, nil
}

func ParseEthLog(in map[string]interface{}) (*ethtypes.EthLog, error) {
	el := &ethtypes.EthLog{}

	ethHash := func(k string, v interface{}) (ethtypes.EthHash, error) {
		s, ok := v.(string)
		if !ok {
			return ethtypes.EthHash{}, xerrors.Errorf(k + " not a string")
		}
		return ethtypes.ParseEthHash(s)
	}

	ethUint64 := func(k string, v interface{}) (ethtypes.EthUint64, error) {
		s, ok := v.(string)
		if !ok {
			return 0, xerrors.Errorf(k + " not a string")
		}
		parsedInt, err := strconv.ParseUint(strings.Replace(s, "0x", "", -1), 16, 64)
		if err != nil {
			return 0, err
		}
		return ethtypes.EthUint64(parsedInt), nil
	}

	var err error
	for k, v := range in {
		switch k {
		case "removed":
			b, ok := v.(bool)
			if ok {
				el.Removed = b
				continue
			}
			s, ok := v.(string)
			if !ok {
				return nil, xerrors.Errorf(k + ": not a string")
			}
			el.Removed, err = strconv.ParseBool(s)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "address":
			s, ok := v.(string)
			if !ok {
				return nil, xerrors.Errorf(k + ": not a string")
			}
			el.Address, err = ethtypes.ParseEthAddress(s)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "logIndex":
			el.LogIndex, err = ethUint64(k, v)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "transactionIndex":
			el.TransactionIndex, err = ethUint64(k, v)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "blockNumber":
			el.BlockNumber, err = ethUint64(k, v)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "transactionHash":
			el.TransactionHash, err = ethHash(k, v)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "blockHash":
			el.BlockHash, err = ethHash(k, v)
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
		case "data":
			s, ok := v.(string)
			if !ok {
				return nil, xerrors.Errorf(k + ": not a string")
			}
			data, err := hex.DecodeString(s[2:])
			if err != nil {
				return nil, xerrors.Errorf("%s: %w", k, err)
			}
			el.Data = data

		case "topics":
			s, ok := v.(string)
			if ok {
				topic, err := hex.DecodeString(s[2:])
				if err != nil {
					return nil, xerrors.Errorf("%s: %w", k, err)
				}
				el.Topics = append(el.Topics, paddedEthHash(topic))
				continue
			}

			sl, ok := v.([]interface{})
			if !ok {
				return nil, xerrors.Errorf(k + ": not a slice")
			}
			for _, s := range sl {
				topic, err := hex.DecodeString(s.(string)[2:])
				if err != nil {
					return nil, xerrors.Errorf("%s: %w", k, err)
				}
				el.Topics = append(el.Topics, paddedEthHash(topic))
			}
		}
	}

	return el, err
}

func paddedUint64(v uint64) ethtypes.EthBytes {
	buf := make([]byte, 32)
	binary.BigEndian.PutUint64(buf[24:], v)
	return buf
}

func uint64EthHash(v uint64) ethtypes.EthHash {
	var buf ethtypes.EthHash
	binary.BigEndian.PutUint64(buf[24:], v)
	return buf
}

func paddedEthHash(orig []byte) ethtypes.EthHash {
	if len(orig) > 32 {
		panic("exceeds EthHash length")
	}
	var ret ethtypes.EthHash
	needed := 32 - len(orig)
	copy(ret[needed:], orig)
	return ret
}

func packUint64Values(vals ...uint64) []byte {
	ret := []byte{}
	for _, v := range vals {
		buf := paddedUint64(v)
		ret = append(ret, buf...)
	}
	return ret
}

func unpackUint64Values(data []byte) []uint64 {
	if len(data)%32 != 0 {
		panic("data length not a multiple of 32")
	}

	var vals []uint64
	for i := 0; i < len(data); i += 32 {
		v := binary.BigEndian.Uint64(data[i+24 : i+32])
		vals = append(vals, v)
	}
	return vals
}
