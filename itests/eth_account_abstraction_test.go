package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin2 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEthAccountAbstraction goes over the placeholder creation and promotion workflow:
// - an placeholder is created when it receives a message
// - the placeholder turns into an EOA when it sends a message
func TestEthAccountAbstraction(t *testing.T) {
	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpKey, err := key.GenerateKey(types.KTDelegated)
	require.NoError(t, err)

	placeholderAddress, err := client.WalletImport(ctx, &secpKey.KeyInfo)
	require.NoError(t, err)

	// create an placeholder actor at the target address
	msgCreatePlaceholder := &types.Message{
		From:  client.DefaultKey.Address,
		To:    placeholderAddress,
		Value: abi.TokenAmount(types.MustParseFIL("100")),
	}
	smCreatePlaceholder, err := client.MpoolPushMessage(ctx, msgCreatePlaceholder, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, smCreatePlaceholder.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)

	require.True(t, mLookup.Receipt.ExitCode.IsSuccess())

	// confirm the placeholder is an placeholder
	placeholderActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, uint64(0), placeholderActor.Nonce)
	require.True(t, builtin.IsPlaceholderActor(placeholderActor.Code))
	require.Equal(t, msgCreatePlaceholder.Value, placeholderActor.Balance)

	// send a message from the placeholder address
	msgFromPlaceholder := &types.Message{
		From: placeholderAddress,
		// self-send because an "eth tx payload" can't be to a filecoin address?
		To:     placeholderAddress,
		Method: builtin2.MethodsEVM.InvokeContract,
	}
	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	txArgs, err := ethtypes.Eth1559TxArgsFromUnsignedFilecoinMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err := txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err := client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err := client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, mLookup.Receipt.ExitCode.IsSuccess())

	// confirm ugly Placeholder duckling has turned into a beautiful EthAccount swan

	eoaActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
	require.Equal(t, uint64(1), eoaActor.Nonce)

	// Send another message, it should succeed without any code CID changes

	msgFromPlaceholder = &types.Message{
		From:   placeholderAddress,
		To:     placeholderAddress,
		Method: builtin2.MethodsEVM.InvokeContract,
		Nonce:  1,
	}

	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	txArgs, err = ethtypes.Eth1559TxArgsFromUnsignedFilecoinMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err = txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err = client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err = client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, mLookup.Receipt.ExitCode.IsSuccess())

	// confirm no changes in code CID

	eoaActor, err = client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, uint64(2), eoaActor.Nonce)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
}

// Tests that an placeholder turns into an EthAccout even if the message fails
func TestEthAccountAbstractionFailure(t *testing.T) {
	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpKey, err := key.GenerateKey(types.KTDelegated)
	require.NoError(t, err)

	placeholderAddress, err := client.WalletImport(ctx, &secpKey.KeyInfo)
	require.NoError(t, err)

	// create a placeholder actor at the target address
	msgCreatePlaceholder := &types.Message{
		From:   client.DefaultKey.Address,
		To:     placeholderAddress,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract,
	}
	smCreatePlaceholder, err := client.MpoolPushMessage(ctx, msgCreatePlaceholder, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, smCreatePlaceholder.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, mLookup.Receipt.ExitCode.IsSuccess())

	// confirm the placeholder is an placeholder
	placeholderActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, uint64(0), placeholderActor.Nonce)
	require.True(t, builtin.IsPlaceholderActor(placeholderActor.Code))
	require.Equal(t, msgCreatePlaceholder.Value, placeholderActor.Balance)

	// send a message from the placeholder address
	msgFromPlaceholder := &types.Message{
		From:   placeholderAddress,
		To:     placeholderAddress,
		Value:  abi.TokenAmount(types.MustParseFIL("20")),
		Method: builtin2.MethodsEVM.InvokeContract,
	}
	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	msgFromPlaceholder.Value = abi.TokenAmount(types.MustParseFIL("1000"))
	txArgs, err := ethtypes.Eth1559TxArgsFromUnsignedFilecoinMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err := txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err := client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err := client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	// message should have failed because we didn't have enough $$$
	require.Equal(t, exitcode.SysErrInsufficientFunds, mLookup.Receipt.ExitCode)

	// BUT, ugly Placeholder duckling should have turned into a beautiful EthAccount swan anyway

	eoaActor, err := client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
	require.Equal(t, uint64(1), eoaActor.Nonce)

	// Send a valid message now, it should succeed without any code CID changes

	msgFromPlaceholder = &types.Message{
		From:   placeholderAddress,
		To:     placeholderAddress,
		Nonce:  1,
		Value:  abi.NewTokenAmount(1),
		Method: builtin2.MethodsEVM.InvokeContract,
	}

	msgFromPlaceholder, err = client.GasEstimateMessageGas(ctx, msgFromPlaceholder, nil, types.EmptyTSK)
	require.NoError(t, err)

	txArgs, err = ethtypes.Eth1559TxArgsFromUnsignedFilecoinMessage(msgFromPlaceholder)
	require.NoError(t, err)

	digest, err = txArgs.ToRlpUnsignedMsg()
	require.NoError(t, err)

	siggy, err = client.WalletSign(ctx, placeholderAddress, digest)
	require.NoError(t, err)

	smFromPlaceholderCid, err = client.MpoolPush(ctx, &types.SignedMessage{Message: *msgFromPlaceholder, Signature: *siggy})
	require.NoError(t, err)

	mLookup, err = client.StateWaitMsg(ctx, smFromPlaceholderCid, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, mLookup.Receipt.ExitCode.IsSuccess())

	// confirm no changes in code CID

	eoaActor, err = client.StateGetActor(ctx, placeholderAddress, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, uint64(2), eoaActor.Nonce)

	require.False(t, builtin.IsPlaceholderActor(eoaActor.Code))
	require.True(t, builtin.IsEthAccountActor(eoaActor.Code))
}

// Tests that f4 addresses that aren't placeholders/ethaccounts can't be top-level senders
func TestEthAccountAbstractionFailsFromEvmActor(t *testing.T) {
	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()

	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	// install a contract from the placeholder
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
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

	client.EVM().SubmitTransaction(ctx, &tx)

	smsg, err := ethtypes.ToSignedFilecoinMessage(&tx)
	require.NoError(t, err)

	ml, err := client.StateWaitMsg(ctx, smsg.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.True(t, ml.Receipt.ExitCode.IsSuccess())

	// Get contract address, assert it's an EVM actor
	contractAddr, err := client.EVM().ComputeContractAddress(ethAddr, 0).ToFilecoinAddress()
	require.NoError(t, err)

	client.AssertActorType(ctx, contractAddr, "evm")

	msgFromContract := &types.Message{
		From: contractAddr,
		To:   contractAddr,
	}

	_, err = client.GasEstimateMessageGas(ctx, msgFromContract, nil, types.EmptyTSK)
	require.Error(t, err, "expected gas estimation to fail")
	require.Contains(t, err.Error(), "SysErrSenderInvalid")
}

func TestEthAccountManagerPermissions(t *testing.T) {
	kit.QuietMiningLogs()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// setup f1/f3/f4 accounts

	wsp, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	wbl, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	wdl, err := client.WalletNew(ctx, types.KTDelegated)
	require.NoError(t, err)

	def := client.DefaultKey.Address

	// send some funds
	client.ExpectSend(ctx, def, wsp, types.FromFil(10), "")
	client.ExpectSend(ctx, def, wbl, types.FromFil(10), "")
	client.ExpectSend(ctx, def, wdl, types.FromFil(10), "")
	require.NoError(t, err)

	// make sure that EAM only allows CreateExternal to be called by accounts
	client.ExpectSend(ctx, wsp, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create, &eam.CreateParams{Nonce: 0}))
	client.ExpectSend(ctx, wbl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create, &eam.CreateParams{Nonce: 0}))
	client.ExpectSend(ctx, wdl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create, &eam.CreateParams{Nonce: 0}))

	client.ExpectSend(ctx, wsp, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create2, &eam.Create2Params{}))
	client.ExpectSend(ctx, wbl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create2, &eam.Create2Params{}))
	client.ExpectSend(ctx, wdl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "not one of supported (18)", client.MakeSendCall(builtin2.MethodsEAM.Create2, &eam.Create2Params{}))

	contractHex, err := os.ReadFile("contracts/SimpleCoin.hex")
	require.NoError(t, err)
	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)
	contractParams := abi.CborBytes(contract)

	client.ExpectSend(ctx, wsp, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "", client.MakeSendCall(builtin2.MethodsEAM.CreateExternal, &contractParams))
	client.ExpectSend(ctx, wbl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "", client.MakeSendCall(builtin2.MethodsEAM.CreateExternal, &contractParams))
	client.ExpectSend(ctx, wdl, builtin2.EthereumAddressManagerActorAddr, big.Zero(), "", client.MakeSendCall(builtin2.MethodsEAM.CreateExternal, &contractParams))
}
