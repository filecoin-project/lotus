package itests

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestGetCodeAndNonce ensures that GetCode and GetTransactionCount return the correct results for:
// 1. Placeholders.
// 2. Non-existent actors.
// 3. Normal EVM actors.
// 4. Self-destructed EVM actors.
func TestGetCodeAndNonce(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Accounts should have empty code, empty nonce.
	{
		// A random eth address should have no code.
		_, ethAddr, filAddr := client.EVM().NewAccount()
		bytecode, err := client.EVM().EthGetCode(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Empty(t, bytecode)

		// Nonce should also be zero
		nonce, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Zero(t, nonce)

		// send some funds to the account.
		kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

		// The code should still be empty, target is now a placeholder.
		bytecode, err = client.EVM().EthGetCode(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Empty(t, bytecode)

		// Nonce should still be zero.
		nonce, err = client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Zero(t, nonce)
	}

	// Check contract code.
	{
		// install a contract
		contractHex, err := os.ReadFile("./contracts/SelfDestruct.hex")
		require.NoError(t, err)
		contract, err := hex.DecodeString(string(contractHex))
		require.NoError(t, err)
		createReturn := client.EVM().DeployContract(ctx, client.DefaultKey.Address, contract)
		contractAddr := createReturn.EthAddress
		contractFilAddr := *createReturn.RobustAddress

		// The newly deployed contract should not be empty.
		bytecode, err := client.EVM().EthGetCode(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.NotEmpty(t, bytecode)

		// Nonce should be one.
		nonce, err := client.EVM().EthGetTransactionCount(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Equal(t, ethtypes.EthUint64(1), nonce)

		// Destroy it.
		_, _, err = client.EVM().InvokeContractByFuncName(ctx, client.DefaultKey.Address, contractFilAddr, "destroy()", nil)
		require.NoError(t, err)

		// The code should be empty again.
		bytecode, err = client.EVM().EthGetCode(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Empty(t, bytecode)

		// Nonce should go back to zero
		nonce, err = client.EVM().EthGetTransactionCount(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Zero(t, nonce)
	}

}
