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

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
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

	client.EVM().SignLegacyHomesteadTransaction(&tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyHomesteadTransaction(&tx, key.PrivateKey)

	hash := client.EVM().SubmitTransaction(ctx, &tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethAddr, receipt.From)
	require.EqualValues(t, ethAddr2, *receipt.To)
	require.EqualValues(t, hash, receipt.TransactionHash)
	require.EqualValues(t, ethtypes.EthLegacyTxType, receipt.Type)

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

func TestLegacyEIP155ValueTransferValidSignatureFailsNV22(t *testing.T) {
	blockTime := 100 * time.Millisecond

	nv23Height := 10
	// We will move to NV23 at epoch 10
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version22,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version23,
		Height:    abi.ChainEpoch(nv23Height),
		Migration: filcns.UpgradeActorsV13,
	}))

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

	legacyTx := &ethtypes.EthLegacyHomesteadTxArgs{
		Value:    big.NewInt(100),
		Nonce:    0,
		To:       &ethAddr2,
		GasPrice: types.NanoFil,
		GasLimit: int(gaslimit),
		V:        big.Zero(),
		R:        big.Zero(),
		S:        big.Zero(),
	}
	tx := ethtypes.NewEthLegacy155TxArgs(legacyTx)

	// TX will fail as we're still at NV22
	client.EVM().SignLegacyEIP155Transaction(tx, key.PrivateKey, big.NewInt(buildconstants.Eip155ChainId))

	signed, err := tx.ToRawTxBytesSigned()
	require.NoError(t, err)

	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "network version should be at least NV23 for sending legacy ETH transactions")
}

func TestLegacyEIP155ValueTransferValidSignature(t *testing.T) {
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

	legacyTx := &ethtypes.EthLegacyHomesteadTxArgs{
		Value:    big.NewInt(100),
		Nonce:    0,
		To:       &ethAddr2,
		GasPrice: types.NanoFil,
		GasLimit: int(gaslimit),
		V:        big.Zero(),
		R:        big.Zero(),
		S:        big.Zero(),
	}
	tx := ethtypes.NewEthLegacy155TxArgs(legacyTx)

	client.EVM().SignLegacyEIP155Transaction(tx, key.PrivateKey, big.NewInt(buildconstants.Eip155ChainId))
	// Mangle signature
	innerTx := tx.GetLegacyTx()
	innerTx.V.Int.Xor(innerTx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRawTxBytesSigned()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature but incorrect chain ID
	client.EVM().SignLegacyEIP155Transaction(tx, key.PrivateKey, big.NewInt(buildconstants.Eip155ChainId))

	signed, err = tx.ToRawTxBytesSigned()
	require.NoError(t, err)

	hash, err := client.EVM().EthSendRawTransaction(ctx, signed)
	require.NoError(t, err)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethAddr, receipt.From)
	require.EqualValues(t, ethAddr2, *receipt.To)
	require.EqualValues(t, hash, receipt.TransactionHash)
	require.EqualValues(t, ethtypes.EthLegacyTxType, receipt.Type)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Validate that we sent the expected transaction.
	ethTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.Nil(t, ethTx.MaxPriorityFeePerGas)
	require.Nil(t, ethTx.MaxFeePerGas)

	innerTx = tx.GetLegacyTx()
	require.EqualValues(t, ethAddr, ethTx.From)
	require.EqualValues(t, ethAddr2, *ethTx.To)
	require.EqualValues(t, innerTx.Nonce, ethTx.Nonce)
	require.EqualValues(t, hash, ethTx.Hash)
	require.EqualValues(t, innerTx.Value, ethTx.Value)
	require.EqualValues(t, 0, ethTx.Type)
	require.EqualValues(t, buildconstants.Eip155ChainId, ethTx.ChainID)
	require.EqualValues(t, ethtypes.EthBytes{}, ethTx.Input)
	require.EqualValues(t, innerTx.GasLimit, ethTx.Gas)
	require.EqualValues(t, innerTx.GasPrice, *ethTx.GasPrice)
	require.EqualValues(t, innerTx.R, ethTx.R)
	require.EqualValues(t, innerTx.S, ethTx.S)
	require.EqualValues(t, innerTx.V, ethTx.V)
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
	tx, err := deployLegacyContractTx(ctx, t, client, ethAddr)
	require.NoError(t, err)

	client.EVM().SignLegacyHomesteadTransaction(tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyHomesteadTransaction(tx, key.PrivateKey)

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

	client.EVM().SignLegacyHomesteadTransaction(&invokeTx, key.PrivateKey)
	// Mangle signature
	invokeTx.V.Int.Xor(invokeTx.V.Int, big.NewInt(1).Int)

	signed, err = invokeTx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignLegacyHomesteadTransaction(&invokeTx, key.PrivateKey)
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

func deployLegacyContractTx(ctx context.Context, t *testing.T, client *kit.TestFullNode, ethAddr ethtypes.EthAddress) (*ethtypes.EthLegacyHomesteadTxArgs, error) {
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
