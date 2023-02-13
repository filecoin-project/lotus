package itests

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestGetCode ensures that GetCode returns the correct results for:
// 1. Placeholders.
// 2. Non-existent actors.
// 3. Normal EVM actors.
// 4. Self-destructed EVM actors.
func TestGetCode(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Accounts should have empty code.
	{
		// A random eth address should have no code.
		_, ethAddr, filAddr := client.EVM().NewAccount()
		bytecode, err := client.EVM().EthGetCode(ctx, ethAddr, "latest")
		require.NoError(t, err)
		require.Empty(t, bytecode)

		// send some funds to the account.
		kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

		// The code should still be empty, target is now a placeholder.
		bytecode, err = client.EVM().EthGetCode(ctx, ethAddr, "latest")
		require.NoError(t, err)
		require.Empty(t, bytecode)
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
		bytecode, err := client.EVM().EthGetCode(ctx, contractAddr, "latest")
		require.NoError(t, err)
		require.NotEmpty(t, bytecode)

		// Destroy it.
		_, _, err = client.EVM().InvokeContractByFuncName(ctx, client.DefaultKey.Address, contractFilAddr, "destroy()", nil)
		require.NoError(t, err)

		// The code should be empty again.
		bytecode, err = client.EVM().EthGetCode(ctx, contractAddr, "latest")
		require.NoError(t, err)
		require.Empty(t, bytecode)
	}

}
