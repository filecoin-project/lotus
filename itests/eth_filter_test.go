// stm: #integration
package itests

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestEthNewPendingTransactionFilter(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.RealTimeFilterAPI())
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

		count := 0
		for {
			select {
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply {
						msgs, err := client.ChainGetMessagesInTipset(ctx, change.Val.Key())
						require.NoError(t, err)
						count += len(msgs)
						if count == iterations {
							waitAllCh <- struct{}{}
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
	case <-time.After(time.Minute):
		t.Errorf("timeout to wait for pack messages")
	}

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(t, err)

	// expect to have seen iteration number of mpool messages
	require.Equal(t, iterations, len(res.Results))
}

func TestEthNewBlockFilter(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.RealTimeFilterAPI())
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
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(t, err)
		<-headChangeCh // skip hccurrent

		count := 0
		for {
			select {
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply || change.Type == store.HCRevert {
						count++
						if count == iterations {
							waitAllCh <- struct{}{}
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
	case <-time.After(time.Minute):
		t.Errorf("timeout to wait for pack messages")
	}

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(t, err)

	// expect to have seen iteration number of tipsets
	require.Equal(t, iterations, len(res.Results))
}

func TestEthNewFilterCatchAll(t *testing.T) {
	require := require.New(t)

	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.RealTimeFilterAPI())
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
	filterID, err := client.EthNewFilter(ctx, &api.EthFilterSpec{})
	require.NoError(err)

	const iterations = 10

	type msgInTipset struct {
		msg api.Message
		ts  *types.TipSet
	}

	msgChan := make(chan msgInTipset, iterations)

	waitAllCh := make(chan struct{})
	go func() {
		headChangeCh, err := client.ChainNotify(ctx)
		require.NoError(err)
		<-headChangeCh // skip hccurrent

		count := 0
		for {
			select {
			case headChanges := <-headChangeCh:
				for _, change := range headChanges {
					if change.Type == store.HCApply || change.Type == store.HCRevert {
						msgs, err := client.ChainGetMessagesInTipset(ctx, change.Val.Key())
						require.NoError(err)

						count += len(msgs)
						for _, m := range msgs {
							select {
							case msgChan <- msgInTipset{msg: m, ts: change.Val}:
							default:
							}
						}

						if count == iterations {
							close(msgChan)
							close(waitAllCh)
							return
						}
					}
				}
			}
		}
	}()

	time.Sleep(blockTime * 6)

	for i := 0; i < iterations; i++ {
		// log a four topic event with data
		ret := client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, []byte{0x00, 0x00, 0x00, 0x02}, nil)
		require.True(ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	}

	select {
	case <-waitAllCh:
	case <-time.After(time.Minute):
		t.Errorf("timeout to wait for pack messages")
	}

	received := make(map[api.EthHash]msgInTipset)
	for m := range msgChan {
		eh, err := api.NewEthHashFromCid(m.msg.Cid)
		require.NoError(err)
		received[eh] = m
	}
	require.Equal(iterations, len(received), "all messages on chain")

	ts, err := client.ChainHead(ctx)
	require.NoError(err)

	raddr, err := client.StateLookupRobustAddress(ctx, idAddr, ts.Key())
	require.NoError(err)

	ethContractAddr, err := api.EthAddressFromFilecoinAddress(raddr)
	require.NoError(err)

	// collect filter results
	res, err := client.EthGetFilterChanges(ctx, filterID)
	require.NoError(err)

	// expect to have seen iteration number of events
	require.Equal(iterations, len(res.Results))

	fmt.Printf("ethAddr=%v\n", ethContractAddr.String())

	for _, r := range res.Results {
		// since response is a union and Go doesn't support them well, go-jsonrpc won't give us typed results
		rc, ok := r.(map[string]interface{})
		require.True(ok, "result type")

		elog, err := ParseEthLog(rc)
		require.NoError(err)

		_ = elog

		require.Equal(ethContractAddr.String(), rc["address"], "event address")
		require.Equal("0x0", rc["transactionIndex"], "transaction index") // only one message per tipset

		txHashAny, ok := rc["transactionHash"]
		require.True(ok, "transactionHash")
		txHashStr, ok := txHashAny.(string)
		require.True(ok, "transactionHash string")

		txHash, err := api.EthHashFromHex(txHashStr)
		require.NoError(err)

		msg, exists := received[txHash]
		require.True(exists, "message seen on chain")

		tsCid, err := msg.ts.Key().Cid()
		require.NoError(err)

		tsCidHash, err := api.NewEthHashFromCid(tsCid)
		require.NoError(err)

		require.Equal(tsCidHash.String(), rc["blockHash"], "block hash")

		for k, v := range rc {
			switch k {
			}
			fmt.Printf("%s=%v\n", k, v)
		}
	}
}

func ParseEthLog(in map[string]interface{}) (*api.EthLog, error) {
	el := &api.EthLog{}

	ethHash := func(k string, v interface{}) (api.EthHash, error) {
		s, ok := v.(string)
		if !ok {
			return api.EthHash{}, xerrors.Errorf(k + " not a string")
		}
		return api.EthHashFromHex(s)
	}

	ethUint64 := func(k string, v interface{}) (api.EthUint64, error) {
		s, ok := v.(string)
		if !ok {
			return 0, xerrors.Errorf(k + " not a string")
		}
		parsedInt, err := strconv.ParseUint(strings.Replace(s, "0x", "", -1), 16, 64)
		if err != nil {
			return 0, err
		}
		return api.EthUint64(parsedInt), nil
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
				return nil, xerrors.Errorf(k + " not a string")
			}
			el.Removed, err = strconv.ParseBool(s)
			if err != nil {
				return nil, err
			}
		case "address":
			s, ok := v.(string)
			if !ok {
				return nil, xerrors.Errorf(k + " not a string")
			}
			el.Address, err = api.EthAddressFromHex(s)
			if err != nil {
				return nil, err
			}
		case "logIndex":
			el.LogIndex, err = ethUint64(k, v)
			if err != nil {
				return nil, err
			}
		case "transactionIndex":
			el.TransactionIndex, err = ethUint64(k, v)
			if err != nil {
				return nil, err
			}
		case "blockNumber":
			el.BlockNumber, err = ethUint64(k, v)
			if err != nil {
				return nil, err
			}
		case "transactionHash":
			el.TransactionHash, err = ethHash(k, v)
			if err != nil {
				return nil, err
			}
		case "blockHash":
			el.BlockHash, err = ethHash(k, v)
			if err != nil {
				return nil, err
			}
		case "data":
			sl, ok := v.([]interface{})
			if !ok {
				return nil, xerrors.Errorf(k + " not a slice")
			}
			for _, s := range sl {
				data, err := ethHash(k, s)
				if err != nil {
					return nil, err
				}
				el.Data = append(el.Data, data)
			}
		case "topics":
			sl, ok := v.([]interface{})
			if !ok {
				return nil, xerrors.Errorf(k + " not a slice")
			}
			for _, s := range sl {
				topic, err := ethHash(k, s)
				if err != nil {
					return nil, err
				}
				el.Topics = append(el.Topics, topic)
			}
		}
	}

	return el, err
}
