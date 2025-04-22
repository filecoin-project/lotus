package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMintContract tests deploying an implementation contract and a proxy contract
// that delegates to it, then mints tokens via the proxy contract.
// The hex files contain compiled copies of https://github.com/recallnet at git hash 06ec52342ffe6cd29cb9c06ebf5a785f4a057c0e
//
// - Proxy contract: 0xf0438cd20Fa4855997297A9C1299469CA10b58bf
// - Implementation contract: 0x1835374384AA51B169c0705DA26A84bB760F2B37
//
// We're using these to reproduce https://github.com/recallnet/contracts/issues/98
func TestMintContract(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(
		t,
		kit.MockProofs(),
		kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Read implementation contract bytecode
	impContractHex, err := os.ReadFile("./contracts/MintImpl.hex")
	require.NoError(t, err)
	implContract, err := hex.DecodeString(string(impContractHex))
	require.NoError(t, err)

	// Create a new Ethereum account for deploying contracts
	key, ethAddr, deployer := client.EVM().NewAccount()

	// Send funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	// Verify balances
	bal := client.EVM().AssertAddressBalanceConsistent(ctx, deployer)
	require.Equal(t, types.FromFil(10), bal)

	// Get gas parameters for implementation deployment
	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: implContract,
	}})
	require.NoError(t, err)

	implGaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	// Deploy implementation contract
	impTx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                0,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(implGaslimit),
		Input:                implContract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&impTx, key.PrivateKey)
	impHash := client.EVM().SubmitTransaction(ctx, &impTx)

	// Wait for implementation contract to be deployed
	impTxReceipt, err := client.EVM().WaitTransaction(ctx, impHash)
	require.NoError(t, err)
	require.NotNil(t, impTxReceipt.ContractAddress, "Implementation contract address is nil")
	impContractAddr := *impTxReceipt.ContractAddress

	// Read proxy contract bytecode
	proxyContractHex, err := os.ReadFile("./contracts/MintProxy.hex")
	require.NoError(t, err)

	// Replace the implementation address in the proxy contract bytecode
	// The placeholder address "1835374384aa51b169c0705da26a84bb760f2b37" needs to be replaced with the actual implementation address
	proxyHexStr := string(proxyContractHex)
	proxyHexStr = strings.Replace(
		proxyHexStr,
		"1835374384aa51b169c0705da26a84bb760f2b37",
		hex.EncodeToString(impContractAddr[:]),
		1,
	)

	proxyContract, err := hex.DecodeString(proxyHexStr)
	require.NoError(t, err)

	// Get gas parameters for proxy deployment
	gasParams, err = json.Marshal(ethtypes.EthEstimateGasParams{
		Tx: ethtypes.EthCall{
			From: &ethAddr,
			Data: proxyContract,
		},
	})
	require.NoError(t, err)

	proxyGaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	// Deploy proxy contract
	proxyTx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                1, // Second transaction from this account
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(proxyGaslimit),
		Input:                proxyContract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&proxyTx, key.PrivateKey)
	proxyHash := client.EVM().SubmitTransaction(ctx, &proxyTx)

	// Wait for proxy contract to be deployed
	proxyTxReceipt, err := client.EVM().WaitTransaction(ctx, proxyHash)
	require.NoError(t, err)
	require.NotNil(t, proxyTxReceipt.ContractAddress, "Proxy contract address is nil")
	proxyContractAddr := *proxyTxReceipt.ContractAddress

	// Call mint function on the proxy contract
	// 40c10f19 = mint(address,uint256)
	// Parameters: address 90f79bf6eb2c4f870365e785982e1f101e93b906 (test address)
	// and amount 8ac7230489e80000 (10 tokens in hex)
	mintParams, err := hex.DecodeString("40c10f1900000000000000000000000090f79bf6eb2c4f870365e785982e1f101e93b9060000000000000000000000000000000000000000000000008ac7230489e80000")
	require.NoError(t, err)

	// Get gas parameters for mint call
	gasParams, err = json.Marshal(ethtypes.EthEstimateGasParams{
		Tx: ethtypes.EthCall{
			From: &ethAddr,
			To:   &proxyContractAddr,
			Data: mintParams,
		},
	})
	require.NoError(t, err)

	mintGaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	// Prepare mint transaction
	mintTx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                2, // Third transaction from this account
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(mintGaslimit),
		To:                   &proxyContractAddr,
		Input:                mintParams,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&mintTx, key.PrivateKey)
	mintHash := client.EVM().SubmitTransaction(ctx, &mintTx)

	// Wait for the mint transaction to be processed
	mintTxReceipt, err := client.EVM().WaitTransaction(ctx, mintHash)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(0x1), mintTxReceipt.Status, "Mint transaction failed")
}
