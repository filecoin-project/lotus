package itests

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestValueTransferValidSignature(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()
	_, ethAddr2, _ := client.EVM().NewAccount()

	kit.SendFunds(ctx, t, client, deployer, types.FromFil(1000))

	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	})
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Value:                big.NewInt(100),
		Nonce:                0,
		To:                   &ethAddr2,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignTransaction(&tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, &tx)

	receipt, err := waitForEthTxReceipt(ctx, client, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	ethTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.Nil(t, err)
	require.EqualValues(t, ethAddr, ethTx.From)
	require.EqualValues(t, ethAddr2, *ethTx.To)
	require.EqualValues(t, tx.ChainID, ethTx.ChainID)
	require.EqualValues(t, tx.Nonce, ethTx.Nonce)
	require.EqualValues(t, hash, ethTx.Hash)
	require.EqualValues(t, tx.Value, ethTx.Value)
	require.EqualValues(t, 2, ethTx.Type)
	require.EqualValues(t, ethtypes.EthBytes{}, ethTx.Input)
	require.EqualValues(t, tx.GasLimit, ethTx.Gas)
	require.EqualValues(t, tx.MaxFeePerGas, ethTx.MaxFeePerGas)
	require.EqualValues(t, tx.MaxPriorityFeePerGas, ethTx.MaxPriorityFeePerGas)
	require.EqualValues(t, tx.V, ethTx.V)
	require.EqualValues(t, tx.R, ethTx.R)
	require.EqualValues(t, tx.S, ethTx.S)
}

func TestLegacyTransaction(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// This is a legacy style transaction obtained from etherscan
	// Tx details: https://etherscan.io/getRawTx?tx=0x0763262208d89efeeb50c8bb05b50c537903fe9d7bdef3b223fd1f5f69f69b32
	txBytes, err := hex.DecodeString("f86f830131cf8504a817c800825208942cf1e5a8250ded8835694ebeb90cfa0237fcb9b1882ec4a5251d1100008026a0f5f8d2244d619e211eeb634acd1bea0762b7b4c97bba9f01287c82bfab73f911a015be7982898aa7cc6c6f27ff33e999e4119d6cd51330353474b98067ff56d930")
	require.NoError(t, err)
	_, err = client.EVM().EthSendRawTransaction(ctx, txBytes)
	require.ErrorContains(t, err, "legacy transaction is not supported")
}

func TestContractDeploymentValidSignature(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

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

	// verify the deployer address is a placeholder.
	client.AssertActorType(ctx, deployer, manifest.PlaceholderKey)

	tx, err := deployContractTx(ctx, client, ethAddr, contract)
	require.NoError(t, err)

	client.EVM().SignTransaction(tx, key.PrivateKey)
	// Mangle signature
	tx.V.Int.Xor(tx.V.Int, big.NewInt(1).Int)

	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignTransaction(tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, tx)

	receipt, err := waitForEthTxReceipt(ctx, client, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Verify that the deployer is now an account.
	client.AssertActorType(ctx, deployer, manifest.EthAccountKey)

	// Verify that the nonce was incremented.
	nonce, err := client.MpoolGetNonce(ctx, deployer)
	require.NoError(t, err)
	require.EqualValues(t, 1, nonce)

	// Verify that the deployer is now an account.
	client.AssertActorType(ctx, deployer, manifest.EthAccountKey)
}

func TestContractInvocation(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

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

	// DEPLOY CONTRACT
	tx, err := deployContractTx(ctx, client, ethAddr, contract)
	require.NoError(t, err)

	client.EVM().SignTransaction(tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, tx)

	receipt, err := waitForEthTxReceipt(ctx, client, hash)
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

	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From: &ethAddr,
		To:   &contractAddr,
		Data: params,
	})
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	invokeTx := ethtypes.EthTxArgs{
		ChainID:              build.Eip155ChainId,
		To:                   &contractAddr,
		Value:                big.Zero(),
		Nonce:                1,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		Input:                params,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&invokeTx, key.PrivateKey)
	// Mangle signature
	invokeTx.V.Int.Xor(invokeTx.V.Int, big.NewInt(1).Int)

	signed, err := invokeTx.ToRlpSignedMsg()
	require.NoError(t, err)
	// Submit transaction with bad signature
	_, err = client.EVM().EthSendRawTransaction(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignTransaction(&invokeTx, key.PrivateKey)
	hash = client.EVM().SubmitTransaction(ctx, &invokeTx)

	receipt, err = waitForEthTxReceipt(ctx, client, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)
}

func TestGetBlockByNumber(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	bms := ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// create a new Ethereum account
	_, ethAddr, filAddr := client.EVM().NewAccount()
	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	latest, err := client.EthBlockNumber(ctx)
	require.NoError(t, err)

	// can get the latest block
	_, err = client.EthGetBlockByNumber(ctx, latest.Hex(), true)
	require.NoError(t, err)

	// fail to get a future block
	_, err = client.EthGetBlockByNumber(ctx, (latest + 10000).Hex(), true)
	require.Error(t, err)

	// inject 10 null rounds
	bms[0].InjectNulls(10)

	// wait until we produce blocks again
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ch, err := client.ChainNotify(tctx)
	require.NoError(t, err)
	<-ch       // current
	hc := <-ch // wait for next block
	require.Equal(t, store.HCApply, hc[0].Type)

	afterNullHeight := hc[0].Val.Height()

	// Fail when trying to fetch a null round.
	_, err = client.EthGetBlockByNumber(ctx, (ethtypes.EthUint64(afterNullHeight - 1)).Hex(), true)
	require.Error(t, err)

	// Fetch balance on a null round; should not fail and should return previous balance.
	// Should be lower than original balance.
	bal, err := client.EthGetBalance(ctx, ethAddr, (ethtypes.EthUint64(afterNullHeight - 1)).Hex())
	require.NoError(t, err)
	require.NotEqual(t, big.Zero(), bal)
	require.Equal(t, types.FromFil(10).Int, bal.Int)
}

func deployContractTx(ctx context.Context, client *kit.TestFullNode, ethAddr ethtypes.EthAddress, contract []byte) (*ethtypes.EthTxArgs, error) {
	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	})
	if err != nil {
		return nil, err
	}

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	if err != nil {
		return nil, err
	}

	// now deploy a contract from the embryo, and validate it went well
	return &ethtypes.EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                0,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		Input:                contract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}, nil
}

func waitForEthTxReceipt(ctx context.Context, client *kit.TestFullNode, hash ethtypes.EthHash) (*api.EthTxReceipt, error) {
	var receipt *api.EthTxReceipt
	var err error
	for i := 0; i < 10000000000; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		if err != nil || receipt == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	return receipt, err
}
