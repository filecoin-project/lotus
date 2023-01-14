package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
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

func TestAddressCreationBeforeDeploy(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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

	// Send contract address some funds

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	sendAmount := big.Div(bal, big.NewInt(2))

	sendMsg := &types.Message{
		From:  fromAddr,
		To:    contractFilAddr,
		Value: sendAmount,
	}
	signedMsg, err := client.MpoolPushMessage(ctx, sendMsg, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, signedMsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	// Check if actor at new address is a placeholder actor
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, builtin.IsPlaceholderActor(actor.Code))

	// Create and deploy evm actor

	method := builtintypes.MethodsEAM.CreateExternal
	contractParams := abi.CborBytes(contract)
	params, err := actors.SerializeParams(&contractParams)
	require.NoError(t, err)

	createMsg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   fromAddr,
		Value:  big.Zero(),
		Method: method,
		Params: params,
	}
	smsg, err := client.MpoolPushMessage(ctx, createMsg, nil)
	require.NoError(t, err)

	wait, err := client.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, wait.Receipt.ExitCode)

	// Check if eth address returned from CreateExternal is the same as eth address predicted at the start
	var createExternalReturn eam.CreateExternalReturn
	err = createExternalReturn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return))
	require.NoError(t, err)

	createdEthAddr, err := ethtypes.CastEthAddress(createExternalReturn.EthAddress[:])
	require.NoError(t, err)
	require.Equal(t, ethAddr, createdEthAddr)

	// Check if newly deployed actor still has funds
	actorPostCreate, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, actorPostCreate.Balance, sendAmount)
	require.True(t, builtin.IsEvmActor(actorPostCreate.Code))

}
