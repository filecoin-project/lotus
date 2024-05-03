package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestLegacyValueTransferValidSignature(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()
	_, ethAddr2, _ := client.EVM().NewAccount()

	kit.SendFunds(ctx, t, client, deployer, types.FromFil(1000))

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From:  &ethAddr,
		To:    &ethAddr2,
		Value: ethtypes.EthBigInt(big.NewInt(100)),
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)
	fmt.Println("gas limit is", gaslimit)

	tx := ethtypes.EthLegacyHomesteadTxArgs{
		Value:    big.NewInt(100),
		Nonce:    0,
		To:       &ethAddr2,
		GasPrice: types.NanoFil,
		GasLimit: int(gaslimit),
		V:        big.Zero(),
		R:        big.Zero(),
		S:        big.Zero(),
	}

	client.EVM().SignLegacyTransaction(&tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyTransaction(&tx, key.PrivateKey)

	hash := client.EVM().SubmitTransaction(ctx, &tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethAddr, receipt.From)
	require.EqualValues(t, ethAddr2, *receipt.To)
	require.EqualValues(t, hash, receipt.TransactionHash)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Validate that we sent the expected transaction.
	ethTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Nil(t, ethTx.MaxPriorityFeePerGas)
	require.Nil(t, ethTx.MaxFeePerGas)

	require.EqualValues(t, ethAddr, ethTx.From)
	require.EqualValues(t, ethAddr2, *ethTx.To)
	require.EqualValues(t, tx.Nonce, ethTx.Nonce)
	require.EqualValues(t, hash, ethTx.Hash)
	require.EqualValues(t, tx.Value, ethTx.Value)
	require.EqualValues(t, 0, ethTx.Type)
	require.EqualValues(t, 0, ethTx.ChainID)
	require.EqualValues(t, ethtypes.EthBytes{}, ethTx.Input)
	require.EqualValues(t, tx.GasLimit, ethTx.Gas)
	require.EqualValues(t, tx.GasPrice, *ethTx.GasPrice)
	require.EqualValues(t, tx.R, ethTx.R)
	require.EqualValues(t, tx.S, ethTx.S)
	require.EqualValues(t, tx.V, ethTx.V)
}

func TestEIP155Tx(t *testing.T) {
	rlpHex := "f86d8083030e1b83291e739490322092a524e0e43a2ec80ec6f35100d24799f28898a7d9b8314c000080820298a0dc782de2fec8cd45e699075beb756ad731943d19a33332fa36e72fd94802ed10a056b3e1e36f2851402661daf0b3284bc8d15db005d1fade908c1117c6ae37429d"
	rlpBytes, err := hex.DecodeString(rlpHex)
	require.NoError(t, err)
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	from := "0xf0a3b487d026406F5ca18891dB6896a3dA900F29"
	ea, err := ethtypes.ParseEthAddress(from)
	require.NoError(t, err)
	fa, err := ea.ToFilecoinAddress()
	require.NoError(t, err)
	fmt.Println("fil addr is", fa.String())
	kit.SendFunds(ctx, t, client, fa, types.FromFil(20000))

	hash, err := client.EthSendRawTransaction(ctx, rlpBytes)
	require.NoError(t, err)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, hash, receipt.TransactionHash)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

}

func TestLegacyContractInvocation(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()
	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	// DEPLOY CONTRACT
	tx, err := deployLegacyContractTx(t, ctx, client, ethAddr)
	require.NoError(t, err)

	client.EVM().SignLegacyTransaction(tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyTransaction(tx, key.PrivateKey)

	hash := client.EVM().SubmitTransaction(ctx, tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Get contract address.
	contractAddr := client.EVM().ComputeContractAddress(ethAddr, 0)

	// INVOKE CONTRACT

	// Params
	// entry point for getBalance - f8b2cb4f
	// address - ff00000000000000000000000000000000000064
	params, err := hex.DecodeString("f8b2cb4f000000000000000000000000ff00000000000000000000000000000000000064")
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		To:   &contractAddr,
		Data: params,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	invokeTx := ethtypes.EthLegacyHomesteadTxArgs{
		To:       &contractAddr,
		Value:    big.Zero(),
		Nonce:    1,
		GasPrice: big.Int(maxPriorityFeePerGas),
		GasLimit: int(gaslimit),
		Input:    params,
		V:        big.Zero(),
		R:        big.Zero(),
		S:        big.Zero(),
	}

	client.EVM().SignLegacyTransaction(&invokeTx, key.PrivateKey)
	// Mangle signature
	invokeTx.V.Int.Xor(invokeTx.V.Int, big.NewInt(1).Int)

	signed, err = invokeTx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyTransaction(&invokeTx, key.PrivateKey)
	hash = client.EVM().SubmitTransaction(ctx, &invokeTx)

	receipt, err = client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Validate that we correctly computed the gas outputs.
	mCid, err := client.EthGetMessageCidByTransactionHash(ctx, &hash)
	require.NoError(t, err)
	require.NotNil(t, mCid)

	invokResult, err := client.StateReplay(ctx, types.EmptyTSK, *mCid)
	require.NoError(t, err)
	require.EqualValues(t, invokResult.GasCost.GasUsed, big.NewInt(int64(receipt.GasUsed)))
	effectiveGasPrice := big.Div(invokResult.GasCost.TotalCost, invokResult.GasCost.GasUsed)
	require.EqualValues(t, effectiveGasPrice, big.Int(receipt.EffectiveGasPrice))
}

func deployLegacyContractTx(t *testing.T, ctx context.Context, client *kit.TestFullNode, ethAddr ethtypes.EthAddress) (*ethtypes.EthLegacyHomesteadTxArgs, error) {
	// install contract
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	if err != nil {
		return nil, err
	}

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	if err != nil {
		return nil, err
	}

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return nil, err
	}

	// now deploy a contract from the embryo, and validate it went well
	return &ethtypes.EthLegacyHomesteadTxArgs{
		Value:    big.Zero(),
		Nonce:    0,
		GasPrice: big.Int(maxPriorityFeePerGas),
		GasLimit: int(gaslimit),
		Input:    contract,
		V:        big.Zero(),
		R:        big.Zero(),
		S:        big.Zero(),
	}, nil
}
