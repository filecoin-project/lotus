package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// TestTransactionHashLookup tests to see if lotus correctly stores a mapping from ethereum transaction hash to
// Filecoin Message Cid
func TestTransactionHashLookup(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()

	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	// now deploy a contract from the embryo, and validate it went well
	tx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                0,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		Input:                contract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&tx, key.PrivateKey)

	rawTxHash, err := tx.TxHash()
	require.NoError(t, err)

	hash := client.EVM().SubmitTransaction(ctx, &tx)
	require.Equal(t, rawTxHash, hash)

	mpoolTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Equal(t, hash, mpoolTx.Hash)

	// Wait for message to land on chain
	var receipt *ethtypes.EthTxReceipt
	for i := 0; i < 20; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		if err != nil || receipt == nil {
			time.Sleep(blocktime)
			continue
		}
		break
	}
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify that the chain transaction now has new fields set.
	chainTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Equal(t, hash, chainTx.Hash)

	// require that the hashes are identical
	require.Equal(t, hash, chainTx.Hash)
	require.NotNil(t, chainTx.BlockNumber)
	require.Greater(t, uint64(*chainTx.BlockNumber), uint64(0))
	require.NotNil(t, chainTx.BlockHash)
	require.NotEmpty(t, *chainTx.BlockHash)
	require.NotNil(t, chainTx.TransactionIndex)
	require.Equal(t, uint64(*chainTx.TransactionIndex), uint64(0)) // only transaction

	// test transaction that doesn't exist, should return nil
	receipt, err = client.EthGetTransactionReceipt(ctx, must.One(ethtypes.ParseEthHash("0x123456789012345678901234567890123456789012345678901234567890123")))
	require.NoError(t, err)
	require.Nil(t, receipt)
}

// TestTransactionHashLookupBlsFilecoinMessage tests to see if lotus can find a BLS Filecoin Message using the transaction hash
func TestTransactionHashLookupBlsFilecoinMessage(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	toSend := big.Div(bal, big.NewInt(2))
	msg := &types.Message{
		From:  client.DefaultKey.Address,
		To:    addr,
		Value: toSend,
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	hash, err := ethtypes.EthHashFromCid(sm.Message.Cid())
	require.NoError(t, err)

	// Assert that BLS messages cannot be retrieved from the message pool until it lands
	// on-chain via the eth API.
	trans, err := client.EthGetTransactionByHash(ctx, &hash)
	require.Nil(t, trans)

	// Now start mining.
	ens.InterconnectAll().BeginMining(blocktime)

	// Wait for message to land on chain
	var receipt *ethtypes.EthTxReceipt
	for i := 0; i < 20; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		if err != nil || receipt == nil {
			time.Sleep(blocktime)
			continue
		}
		break
	}
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, hash, receipt.TransactionHash)

	// Verify that the chain transaction now has new fields set.
	chainTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Equal(t, hash, chainTx.Hash)

	// require that the hashes are identical
	require.Equal(t, hash, chainTx.Hash)
	require.NotNil(t, chainTx.BlockNumber)
	require.Greater(t, uint64(*chainTx.BlockNumber), uint64(0))
	require.NotNil(t, chainTx.BlockHash)
	require.NotEmpty(t, *chainTx.BlockHash)
	require.NotNil(t, chainTx.TransactionIndex)
	require.Equal(t, uint64(*chainTx.TransactionIndex), uint64(0)) // only transaction

	// verify that we correctly reported the to address.
	toId, err := client.StateLookupID(ctx, addr, types.EmptyTSK)
	require.NoError(t, err)

	fp := new(ethtypes.FilecoinAddressToEthAddressParams)
	fp.FilecoinAddress = toId
	params, err := json.Marshal(fp)
	require.NoError(t, err)

	toEth, err := client.FilecoinAddressToEthAddress(ctx, params)
	require.NoError(t, err)
	require.Equal(t, &toEth, chainTx.To)

	const expectedHex = "868e10c4" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000060" +
		"0000000000000000000000000000000000000000000000000000000000000000"

	// verify that the params are correctly encoded.
	expected, err := hex.DecodeString(expectedHex)
	require.NoError(t, err)

	require.Equal(t, ethtypes.EthBytes(expected), chainTx.Input)
}

// TestTransactionHashLookupSecpFilecoinMessage tests to see if lotus can find a Secp Filecoin Message using the transaction hash
func TestTransactionHashLookupSecpFilecoinMessage(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	toSend := big.Div(bal, big.NewInt(2))
	setupMsg := &types.Message{
		From:  client.DefaultKey.Address,
		To:    addr,
		Value: toSend,
	}

	setupSmsg, err := client.MpoolPushMessage(ctx, setupMsg, nil)
	require.NoError(t, err)

	_, err = client.StateWaitMsg(ctx, setupSmsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	// Send message for secp account
	secpMsg := &types.Message{
		From:  addr,
		To:    client.DefaultKey.Address,
		Value: big.Div(toSend, big.NewInt(2)),
	}

	secpSmsg, err := client.MpoolPushMessage(ctx, secpMsg, nil)
	require.NoError(t, err)

	hash, err := ethtypes.EthHashFromCid(secpSmsg.Cid())
	require.NoError(t, err)

	_, err = client.StateWaitMsg(ctx, secpSmsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	receipt, err := client.EthGetTransactionReceipt(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, hash, receipt.TransactionHash)

	// Verify that the chain transaction now has new fields set.
	chainTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Equal(t, hash, chainTx.Hash)

	// require that the hashes are identical
	require.Equal(t, hash, chainTx.Hash)
	require.NotNil(t, chainTx.BlockNumber)
	require.Greater(t, uint64(*chainTx.BlockNumber), uint64(0))
	require.NotNil(t, chainTx.BlockHash)
	require.NotEmpty(t, *chainTx.BlockHash)
	require.NotNil(t, chainTx.TransactionIndex)
	require.Equal(t, uint64(*chainTx.TransactionIndex), uint64(0)) // only transaction

	// verify that we correctly reported the to address.
	toId, err := client.StateLookupID(ctx, client.DefaultKey.Address, types.EmptyTSK)
	require.NoError(t, err)

	fp := new(ethtypes.FilecoinAddressToEthAddressParams)
	fp.FilecoinAddress = toId
	params, err := json.Marshal(fp)
	require.NoError(t, err)

	toEth, err := client.FilecoinAddressToEthAddress(ctx, params)
	require.NoError(t, err)
	require.Equal(t, &toEth, chainTx.To)

	const expectedHex = "868e10c4" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000060" +
		"0000000000000000000000000000000000000000000000000000000000000000"

	// verify that the params are correctly encoded.
	expected, err := hex.DecodeString(expectedHex)
	require.NoError(t, err)

	require.Equal(t, ethtypes.EthBytes(expected), chainTx.Input)
}

// TestTransactionHashLookupNonexistentMessage tests to see if lotus can find a Secp Filecoin Message using the transaction hash
func TestTransactionHashLookupNonexistentMessage(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cid := cid.MustParse("bafy2bzacecapjnxnyw4talwqv5ajbtbkzmzqiosztj5cb3sortyp73ndjl76e")

	// We shouldn't be able to return a hash for this fake cid
	chainHash, err := client.EthGetTransactionHashByCid(ctx, cid)
	require.NoError(t, err)
	require.Nil(t, chainHash)

	calculatedHash, err := ethtypes.EthHashFromCid(cid)
	require.NoError(t, err)

	// We shouldn't be able to return a cid for this fake hash
	chainCid, err := client.EthGetMessageCidByTransactionHash(ctx, &calculatedHash)
	require.NoError(t, err)
	require.Nil(t, chainCid)
}

func TestEthGetMessageCidByTransactionHashEthTx(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()

	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	// now deploy a contract from the embryo, and validate it went well
	tx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                0,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		Input:                contract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&tx, key.PrivateKey)

	sender, err := tx.Sender()
	require.NoError(t, err)

	unsignedMessage, err := tx.ToUnsignedFilecoinMessage(sender)
	require.NoError(t, err)

	rawTxHash, err := tx.TxHash()
	require.NoError(t, err)

	hash := client.EVM().SubmitTransaction(ctx, &tx)
	require.Equal(t, rawTxHash, hash)

	mpoolCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, mpoolCid)

	mpoolTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, mpoolTx)
	require.Equal(t, *unsignedMessage, *mpoolTx)

	// Wait for message to land on chain
	var receipt *ethtypes.EthTxReceipt
	for i := 0; i < 20; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		if err != nil || receipt == nil {
			time.Sleep(blocktime)
			continue
		}
		break
	}
	require.NoError(t, err)
	require.NotNil(t, receipt)

	chainCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, chainCid)

	chainTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, chainTx)
	require.Equal(t, *unsignedMessage, *chainTx)
}

func TestEthGetMessageCidByTransactionHashSecp(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	toSend := big.Div(bal, big.NewInt(2))
	setupMsg := &types.Message{
		From:  client.DefaultKey.Address,
		To:    addr,
		Value: toSend,
	}

	setupSmsg, err := client.MpoolPushMessage(ctx, setupMsg, nil)
	require.NoError(t, err)

	_, err = client.StateWaitMsg(ctx, setupSmsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	// Send message for secp account
	secpMsg := &types.Message{
		From:  addr,
		To:    client.DefaultKey.Address,
		Value: big.Div(toSend, big.NewInt(2)),
	}

	secpSmsg, err := client.MpoolPushMessage(ctx, secpMsg, nil)
	require.NoError(t, err)

	hash, err := ethtypes.EthHashFromCid(secpSmsg.Cid())
	require.NoError(t, err)

	mpoolCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, mpoolCid)

	mpoolTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, mpoolTx)
	require.Equal(t, secpSmsg.Message, *mpoolTx)

	_, err = client.StateWaitMsg(ctx, secpSmsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	chainCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, chainCid)

	chainTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, chainTx)
	require.Equal(t, secpSmsg.Message, *chainTx)
}

func TestEthGetMessageCidByTransactionHashBLS(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// get the existing balance from the default wallet to then split it.
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// create a new address where to send funds.
	addr, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	toSend := big.Div(bal, big.NewInt(2))
	msg := &types.Message{
		From:  client.DefaultKey.Address,
		To:    addr,
		Value: toSend,
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	hash, err := ethtypes.EthHashFromCid(sm.Cid())
	require.NoError(t, err)

	mpoolCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, mpoolCid)

	mpoolTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, mpoolTx)
	require.Equal(t, sm.Message, *mpoolTx)

	_, err = client.StateWaitMsg(ctx, sm.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	chainCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, chainCid)

	chainTx, err := client.ChainGetMessage(ctx, *mpoolCid)
	require.NoError(t, err)
	require.NotNil(t, chainTx)
	require.Equal(t, sm.Message, *chainTx)
}
