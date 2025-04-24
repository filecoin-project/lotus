package itests

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func effectiveEthAddressForCreate(t *testing.T, sender address.Address) ethtypes.EthAddress {
	switch sender.Protocol() {
	case address.SECP256K1, address.BLS:
		hasher := sha3.NewLegacyKeccak256()
		hasher.Write(sender.Bytes())
		addr, err := ethtypes.CastEthAddress(hasher.Sum(nil)[12:])
		require.NoError(t, err)
		return addr
	case address.Delegated:
		addr, err := ethtypes.EthAddressFromFilecoinAddress(sender)
		require.NoError(t, err)
		return addr
	default:
		require.FailNow(t, "unsupported protocol %d", sender.Protocol())
	}
	panic("unreachable")
}

func createAndDeploy(ctx context.Context, t *testing.T, client *kit.TestFullNode, fromAddr address.Address, contract []byte) *ethtypes.EthTxReceipt {
	// Create and deploy evm actor

	method := builtintypes.MethodsEAM.CreateExternal
	contractParams := abi.CborBytes(contract)
	params, actorsErr := actors.SerializeParams(&contractParams)
	require.NoError(t, actorsErr)

	createMsg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   fromAddr,
		Value:  big.Zero(),
		Method: method,
		Params: params,
	}
	smsg, err := client.MpoolPushMessage(ctx, createMsg, nil)
	require.NoError(t, err)

	txHash, err := client.EthGetTransactionHashByCid(ctx, smsg.Cid())
	require.NoError(t, err)

	receipt, err := client.EVM().WaitTransaction(ctx, *txHash)
	require.NoError(t, err)
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)
	return receipt
}

func TestAddressCreationBeforeDeploy(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// We hash the f1/f3 address into the EVM's address space when deploying contracts from
	// accounts.
	effectiveEvmAddress := effectiveEthAddressForCreate(t, fromAddr)
	ethAddr := client.EVM().ComputeContractAddress(effectiveEvmAddress, 1)

	contractFilAddr, err := ethAddr.ToFilecoinAddress()
	require.NoError(t, err)

	//transfer half the wallet balance
	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	sendAmount := big.Div(bal, big.NewInt(2))
	client.EVM().TransferValueOrFail(ctx, fromAddr, contractFilAddr, sendAmount)

	// Check if actor at new address is a placeholder actor
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, builtin.IsPlaceholderActor(actor.Code))

	// Create and deploy evm actor
	receipt := createAndDeploy(ctx, t, client, fromAddr, contract)

	// Check if eth address returned from CreateExternal is the same as eth address predicted at
	// the start
	require.Equal(t, &ethAddr, receipt.ContractAddress)

	// Check if newly deployed actor still has funds
	actorPostCreate, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, actorPostCreate.Balance, sendAmount)
	require.True(t, builtin.IsEvmActor(actorPostCreate.Code))

}

func TestDeployAddressMultipleTimes(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// We hash the f1/f3 address into the EVM's address space when deploying contracts from
	// accounts.
	effectiveEvmAddress := effectiveEthAddressForCreate(t, fromAddr)
	ethAddr := client.EVM().ComputeContractAddress(effectiveEvmAddress, 1)

	contractFilAddr, err := ethAddr.ToFilecoinAddress()
	require.NoError(t, err)

	// Send contract address small funds to init
	sendAmount := big.NewInt(2)
	client.EVM().TransferValueOrFail(ctx, fromAddr, contractFilAddr, sendAmount)

	// Check if actor at new address is a placeholder actor
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, builtin.IsPlaceholderActor(actor.Code))

	// Create and deploy evm actor
	receipt := createAndDeploy(ctx, t, client, fromAddr, contract)

	// Check if eth address returned from CreateExternal is the same as eth address predicted at
	// the start
	require.Equal(t, &ethAddr, receipt.ContractAddress)

	// Check if newly deployed actor still has funds
	actorPostCreate, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, actorPostCreate.Balance, sendAmount)
	require.True(t, builtin.IsEvmActor(actorPostCreate.Code))

	// Create and deploy evm actor
	receipt = createAndDeploy(ctx, t, client, fromAddr, contract)

	// Check that this time eth address returned from CreateExternal is not the same as eth address predicted at the start
	require.NotEqual(t, &ethAddr, receipt.ContractAddress)

}
