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
	"github.com/filecoin-project/lotus/node/config"
)

// TestDeployment smoke tests the deployment of a contract via the
// Ethereum JSON-RPC endpoint, from an EEOA.
func TestDeployment(t *testing.T) {
	// TODO the contract installation and invocation can be lifted into utility methods
	// He who writes the second test, shall do that.
	// kit.QuietMiningLogs()

	// reasonable blocktime so that the tx sits in the mpool for a bit during the test.
	// although this is non-deterministic...
	blockTime := 1 * time.Second
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.WithCfgOpt(func(cfg *config.FullNode) error {
			cfg.ActorEvent.EnableRealTimeFilterAPI = true
			return nil
		}),
	)
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
	bal := client.EVM().AssertAddressBalanceConsistent(ctx, deployer)
	require.Equal(t, types.FromFil(10), bal)

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

	client.EVM().SignTransaction(&tx, key.PrivateKey)

	pendingFilter, err := client.EthNewPendingTransactionFilter(ctx)
	require.NoError(t, err)

	hash := client.EVM().SubmitTransaction(ctx, &tx)
	fmt.Println(hash)

	mpoolTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)

	// require that the hashes are identical
	require.Equal(t, hash, mpoolTx.Hash)

	// these fields should be nil because the tx hasn't landed on chain.
	require.Nil(t, mpoolTx.BlockNumber)
	require.Nil(t, mpoolTx.BlockHash)
	require.Nil(t, mpoolTx.TransactionIndex)

	changes, err := client.EthGetFilterChanges(ctx, pendingFilter)
	require.NoError(t, err)
	require.Len(t, changes.Results, 1)
	require.Equal(t, hash.String(), changes.Results[0])

	var receipt *api.EthTxReceipt
	for i := 0; i < 20; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
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

	// Verify that the chain transaction now has new fields set.
	chainTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)

	// require that the hashes are identical
	require.Equal(t, hash, chainTx.Hash)
	require.NotNil(t, chainTx.BlockNumber)
	require.Greater(t, uint64(*chainTx.BlockNumber), uint64(0))
	require.NotNil(t, chainTx.BlockHash)
	require.NotEmpty(t, *chainTx.BlockHash)
	require.NotNil(t, chainTx.TransactionIndex)
	require.Equal(t, uint64(*chainTx.TransactionIndex), uint64(0)) // only transaction

	// Verify that the deployer is now an account.
	client.AssertActorType(ctx, deployer, manifest.EthAccountKey)

	// Verify that the nonce was incremented.
	nonce, err := client.MpoolGetNonce(ctx, deployer)
	require.NoError(t, err)
	require.EqualValues(t, 1, nonce)

	// Verify that the deployer is now an account.
	client.AssertActorType(ctx, deployer, manifest.EthAccountKey)

	// Get contract address.
	contractAddr, err := client.EVM().ComputeContractAddress(ethAddr, 0).ToFilecoinAddress()
	require.NoError(t, err)

	client.AssertActorType(ctx, contractAddr, "evm")
}
