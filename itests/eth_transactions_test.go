package itests

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
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
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.bin")
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

	unsigned, err := tx.ToRlpUnsignedMsg()
	require.NoError(t, err)
	// Submit transaction without signing message
	hash, err := client.EVM().EthSendRawTransaction(ctx, unsigned)
	require.Error(t, err)

	client.EVM().SignTransaction(&tx, key.PrivateKey)

	hash = client.EVM().SubmitTransaction(ctx, &tx)
	fmt.Println(hash)

	var receipt *api.EthTxReceipt
	for i := 0; i < 10000000000; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		fmt.Println(receipt, err)
		if err != nil || receipt == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)
}

func TestLegacyTransaction(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	txBytes, err := hex.DecodeString("f86f830131cf8504a817c800825208942cf1e5a8250ded8835694ebeb90cfa0237fcb9b1882ec4a5251d1100008026a0f5f8d2244d619e211eeb634acd1bea0762b7b4c97bba9f01287c82bfab73f911a015be7982898aa7cc6c6f27ff33e999e4119d6cd51330353474b98067ff56d930")
	require.NoError(t, err)
	_, err = client.EVM().EthSendRawTransaction(ctx, txBytes)
	require.Errorf(t, err, "legacy transaction is not supported")
}

func TestContractDeploymentValidSignature(t *testing.T) {

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.bin")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()

	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	// verify balances.
	//bal := client.EVM().AssertAddressBalanceConsistent(ctx, deployer)
	//require.Equal(t, types.FromFil(10), bal)

	// verify the deployer address is an embryo.
	client.AssertActorType(ctx, deployer, manifest.EmbryoKey)

	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	})
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	// now deploy a contract from the embryo, and validate it went well
	tx := ethtypes.EthTxArgs{
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
	}

	unsigned, err := tx.ToRlpUnsignedMsg()
	require.NoError(t, err)
	// Submit transaction without signing message
	hash, err := client.EVM().EthSendRawTransaction(ctx, unsigned)
	require.Error(t, err)

	client.EVM().SignTransaction(&tx, key.PrivateKey)

	hash = client.EVM().SubmitTransaction(ctx, &tx)
	fmt.Println(hash)

	var receipt *api.EthTxReceipt
	for i := 0; i < 10000000000; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		fmt.Println(receipt, err)
		if err != nil || receipt == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
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
