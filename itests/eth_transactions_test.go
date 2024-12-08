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

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin2 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
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

	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")
	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{
		Tx: ethtypes.EthCall{
			From: &ethAddr,
			Data: contract,
		},
		BlkParam: &blkParam,
	})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
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

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethAddr, receipt.From)
	require.EqualValues(t, ethAddr2, *receipt.To)
	require.EqualValues(t, hash, receipt.TransactionHash)
	require.EqualValues(t, ethtypes.EIP1559TxType, receipt.Type)

	// Success.
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Validate that we sent the expected transaction.
	ethTx, err := client.EthGetTransactionByHash(ctx, &hash)
	require.NoError(t, err)
	require.EqualValues(t, ethAddr, ethTx.From)
	require.EqualValues(t, ethAddr2, *ethTx.To)
	require.EqualValues(t, tx.ChainID, ethTx.ChainID)
	require.EqualValues(t, tx.Nonce, ethTx.Nonce)
	require.EqualValues(t, hash, ethTx.Hash)
	require.EqualValues(t, tx.Value, ethTx.Value)
	require.EqualValues(t, 2, ethTx.Type)
	require.EqualValues(t, ethtypes.EthBytes{}, ethTx.Input)
	require.EqualValues(t, tx.GasLimit, ethTx.Gas)
	require.EqualValues(t, tx.MaxFeePerGas, *ethTx.MaxFeePerGas)
	require.EqualValues(t, tx.MaxPriorityFeePerGas, *ethTx.MaxPriorityFeePerGas)
	require.EqualValues(t, tx.V, ethTx.V)
	require.EqualValues(t, tx.R, ethTx.R)
	require.EqualValues(t, tx.S, ethTx.S)
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

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
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

	invokeTx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
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
	_, err = client.EVM().EthSendRawTransactionUntrusted(ctx, signed)
	require.Error(t, err)

	// Submit transaction with valid signature
	client.EVM().SignTransaction(&invokeTx, key.PrivateKey)
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

func TestContractInvocationMultiple(t *testing.T) {
	const (
		blockTime     = 100 * time.Millisecond
		totalMessages = 20
		maxUntrusted  = 10
	)

	for _, untrusted := range []bool{true, false} {
		t.Run(fmt.Sprintf("untrusted=%t", untrusted), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
			t.Cleanup(func() {
				_ = client.Stop(ctx)
				_ = miner.Stop(ctx)
			})
			ens.InterconnectAll().BeginMining(blockTime)

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
			contractTx, err := deployContractTx(ctx, client, ethAddr, contract)
			require.NoError(t, err)
			client.EVM().SignTransaction(contractTx, key.PrivateKey)
			deployHash := client.EVM().SubmitTransaction(ctx, contractTx)

			receipt, err := client.EVM().WaitTransaction(ctx, deployHash)
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

			hashes := make([]ethtypes.EthHash, 0)
			baseMsg := ethtypes.Eth1559TxArgs{
				ChainID:              buildconstants.Eip155ChainId,
				To:                   &contractAddr,
				Value:                big.Zero(),
				MaxFeePerGas:         types.NanoFil,
				MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
				GasLimit:             int(gaslimit),
				Input:                params,
				V:                    big.Zero(),
				R:                    big.Zero(),
				S:                    big.Zero(),
			}

			for i := 0; i < totalMessages; i++ {
				invokeTx := baseMsg
				invokeTx.Nonce = i + 1

				client.EVM().SignTransaction(&invokeTx, key.PrivateKey)
				signed, err := invokeTx.ToRlpSignedMsg()
				require.NoError(t, err)

				if untrusted {
					hash, err := client.EVM().EthSendRawTransactionUntrusted(ctx, signed)
					if i >= maxUntrusted {
						require.Error(t, err)
						require.Contains(t, err.Error(), "too many pending messages")
						break
					}
					require.NoError(t, err)
					hashes = append(hashes, hash)
				} else {
					hash, err := client.EVM().EthSendRawTransaction(ctx, signed)
					require.NoError(t, err)
					hashes = append(hashes, hash)
				}
			}

			for _, hash := range hashes {
				receipt, err = client.EVM().WaitTransaction(ctx, hash)
				require.NoError(t, err)
				require.NotNil(t, receipt)
				// Success.
				require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)
			}
		})
	}
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

	nullHeight := afterNullHeight - 1
	for nullHeight > 0 {
		ts, err := client.ChainGetTipSetByHeight(ctx, nullHeight, types.EmptyTSK)
		require.NoError(t, err)
		if ts.Height() == nullHeight {
			nullHeight--
		} else {
			break
		}
	}

	// Fail when trying to fetch a null round.
	_, err = client.EthGetBlockByNumber(ctx, (ethtypes.EthUint64(nullHeight)).Hex(), true)
	require.Error(t, err)

	// Fetch balance on a null round; should not fail and should return previous balance.
	bal, err := client.EthGetBalance(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromNumber(ethtypes.EthUint64(nullHeight)))
	require.NoError(t, err)
	require.NotEqual(t, big.Zero(), bal)
	require.Equal(t, types.FromFil(10).Int, bal.Int)
}

func deployContractTx(ctx context.Context, client *kit.TestFullNode, ethAddr ethtypes.EthAddress, contract []byte) (*ethtypes.Eth1559TxArgs, error) {
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
	return &ethtypes.Eth1559TxArgs{
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
	}, nil
}

// Invoke a contract with empty input.
func TestEthTxFromNativeAccount_EmptyInput(t *testing.T) {
	blockTime := 10 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpAddr, err := address.NewSecp256k1Address([]byte("foobar"))
	require.NoError(t, err)

	msg := &types.Message{
		From:   client.DefaultKey.Address,
		To:     secpAddr,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract,
	}

	sMsg, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)
	client.WaitMsg(ctx, sMsg.Cid())

	hash, err := client.EthGetTransactionHashByCid(ctx, sMsg.Cid())
	require.NoError(t, err)
	tx, err := client.EthGetTransactionByHash(ctx, hash)
	require.NoError(t, err)

	// Expect empty input params given that we "invoked" the contract (well, invoked ourselves).
	require.Equal(t, ethtypes.EthBytes{}, tx.Input)

	// Validate the to/from addresses.
	toId, err := client.StateLookupID(ctx, msg.To, types.EmptyTSK)
	require.NoError(t, err)
	fromId, err := client.StateLookupID(ctx, msg.From, types.EmptyTSK)
	require.NoError(t, err)

	expectedTo, err := ethtypes.EthAddressFromFilecoinAddress(toId)
	require.NoError(t, err)
	expectedFrom, err := ethtypes.EthAddressFromFilecoinAddress(fromId)
	require.NoError(t, err)
	require.Equal(t, &expectedTo, tx.To)
	require.Equal(t, expectedFrom, tx.From)
}

// Invoke a contract with non-empty input.
func TestEthTxFromNativeAccount_NonEmptyInput(t *testing.T) {
	blockTime := 10 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	msg := &types.Message{
		From:   client.DefaultKey.Address,
		To:     client.DefaultKey.Address,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract,
	}

	var err error
	input := abi.CborBytes([]byte{0x1, 0x2, 0x3, 0x4})
	msg.Params, err = actors.SerializeParams(&input)
	require.NoError(t, err)

	sMsg, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)
	client.WaitMsg(ctx, sMsg.Cid())
	hash, err := client.EthGetTransactionHashByCid(ctx, sMsg.Cid())
	require.NoError(t, err)
	tx, err := client.EthGetTransactionByHash(ctx, hash)
	require.NoError(t, err)

	// Expect the decoded input.
	require.EqualValues(t, input, tx.Input)
}

// Invoke a contract, but with incorrectly encoded input. We expect this to be abi-encoded as if it
// were any other method call.
func TestEthTxFromNativeAccount_BadInput(t *testing.T) {
	blockTime := 10 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	msg := &types.Message{
		From:   client.DefaultKey.Address,
		To:     client.DefaultKey.Address,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract,
		Params: []byte{0x1, 0x2, 0x3, 0x4},
	}

	sMsg, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)
	client.WaitMsg(ctx, sMsg.Cid())
	hash, err := client.EthGetTransactionHashByCid(ctx, sMsg.Cid())
	require.NoError(t, err)
	tx, err := client.EthGetTransactionByHash(ctx, hash)
	require.NoError(t, err)

	const expectedHex1 = "868e10c4" + // "handle filecoin method" function selector
		// InvokeEVM method number
		"00000000000000000000000000000000000000000000000000000000e525aa15" +
		// CBOR multicodec (0x51)
		"0000000000000000000000000000000000000000000000000000000000000051" +
		// Offset
		"0000000000000000000000000000000000000000000000000000000000000060" +
		// Number of bytes in the input (4)
		"0000000000000000000000000000000000000000000000000000000000000004" +
		// Input: 1, 2, 3, 4
		"0102030400000000000000000000000000000000000000000000000000000000"

	input, err := hex.DecodeString(expectedHex1)
	require.NoError(t, err)
	require.EqualValues(t, input, tx.Input)

}

// Invoke a native method.
func TestEthTxFromNativeAccount_NativeMethod(t *testing.T) {
	blockTime := 10 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	msg := &types.Message{
		From:   client.DefaultKey.Address,
		To:     client.DefaultKey.Address,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract + 1,
		Params: []byte{0x1, 0x2, 0x3, 0x4},
	}

	sMsg, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)
	client.WaitMsg(ctx, sMsg.Cid())
	hash, err := client.EthGetTransactionHashByCid(ctx, sMsg.Cid())
	require.NoError(t, err)
	tx, err := client.EthGetTransactionByHash(ctx, hash)
	require.NoError(t, err)

	const expectedHex = "868e10c4" + // "handle filecoin method" function selector
		// InvokeEVM+1
		"00000000000000000000000000000000000000000000000000000000e525aa16" +
		// CBOR multicodec (0x51)
		"0000000000000000000000000000000000000000000000000000000000000051" +
		// Offset
		"0000000000000000000000000000000000000000000000000000000000000060" +
		// Number of bytes in the input (4)
		"0000000000000000000000000000000000000000000000000000000000000004" +
		// Input: 1, 2, 3, 4
		"0102030400000000000000000000000000000000000000000000000000000000"
	input, err := hex.DecodeString(expectedHex)
	require.NoError(t, err)
	require.EqualValues(t, input, tx.Input)
}

// Send to an invalid receiver. We're checking to make sure we correctly set `txn.To` to the special
// "reverted" eth addr.
func TestEthTxFromNativeAccount_InvalidReceiver(t *testing.T) {
	blockTime := 10 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	to, err := address.NewActorAddress([]byte("foobar"))
	require.NoError(t, err)

	msg := &types.Message{
		From:   client.DefaultKey.Address,
		To:     to,
		Value:  abi.TokenAmount(types.MustParseFIL("100")),
		Method: builtin2.MethodsEVM.InvokeContract + 1,
		Params: []byte{0x1, 0x2, 0x3, 0x4},
		// We can't estimate gas for a failed message, so we hard-code these values.
		GasLimit:  10_000_000,
		GasFeeCap: abi.NewTokenAmount(10000),
	}

	// We expect the "to" address to be the special "reverted" eth address.
	expectedTo, err := ethtypes.ParseEthAddress("ff0000000000000000000000ffffffffffffffff")
	require.NoError(t, err)

	sMsg, err := client.WalletSignMessage(ctx, client.DefaultKey.Address, msg)
	require.NoError(t, err)
	k, err := client.MpoolPush(ctx, sMsg)
	require.NoError(t, err)
	res, err := client.StateWaitMsg(ctx, k, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, res.Receipt.ExitCode, exitcode.SysErrInvalidReceiver)

	hash, err := client.EthGetTransactionHashByCid(ctx, k)
	require.NoError(t, err)
	tx, err := client.EthGetTransactionByHash(ctx, hash)
	require.NoError(t, err)
	require.EqualValues(t, &expectedTo, tx.To)
}

func TestTraceTransaction(t *testing.T) {
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

	// EthTraceTransaction errors when tx hash is not found
	nonExistentTxHash := "0x0000000000000000000000000000000000000000000000000000000000000000"
	traces, err := client.EthTraceTransaction(ctx, nonExistentTxHash)
	require.Error(t, err)
	require.Contains(t, err.Error(), "transaction not found")
	require.Nil(t, traces)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// get trace and verify values
	traces, err = client.EthTraceTransaction(ctx, hash.String())
	require.NoError(t, err)
	require.NotNil(t, traces)
	require.EqualValues(t, traces[0].TransactionHash, hash)
	require.EqualValues(t, traces[0].BlockNumber, receipt.BlockNumber)
}

func TestTraceFilter(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	bms := ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Example of creating and submitting a transaction
	// Replace with actual test setup as needed
	contractHex, err := os.ReadFile("./contracts/SimpleCoin.hex")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	key, ethAddr, deployer := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	tx, err := deployContractTx(ctx, client, ethAddr, contract)
	require.NoError(t, err)

	client.EVM().SignTransaction(tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, tx)

	// Wait for the transaction to be mined
	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	// Get contract address.
	contractAddr := client.EVM().ComputeContractAddress(ethAddr, 0)

	// get trace and verify values
	tracesx, err := client.EthTraceTransaction(ctx, hash.String())
	require.NoError(t, err)
	require.NotNil(t, tracesx)
	require.EqualValues(t, tracesx[0].TransactionHash, hash)
	require.EqualValues(t, tracesx[0].BlockNumber, receipt.BlockNumber)

	_ = client.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(receipt.BlockNumber+1)))

	// Define filter criteria
	fromBlock := "0x1"
	toBlock := fmt.Sprint(receipt.BlockNumber)
	filter := ethtypes.EthTraceFilterCriteria{
		FromBlock: &fromBlock,
		ToBlock:   &toBlock,
	}

	// Trace filter should find the transaction
	traces, err := client.EthTraceFilter(ctx, filter)
	require.NoError(t, err)
	require.NotNil(t, traces)
	require.NotEmpty(t, traces)

	for i, trace := range traces {
		t.Logf("Trace %d: TransactionPosition=%d, TransactionHash=%s, Type=%s", i, trace.TransactionPosition, trace.TransactionHash, trace.EthTrace.Type)
	}

	// Assert that initial transactions returned by the trace are valid
	require.Len(t, traces, 3)
	require.Equal(t, 1, traces[0].TransactionPosition)
	require.Equal(t, "call", traces[0].EthTrace.Type)
	require.Equal(t, 1, traces[1].TransactionPosition)
	require.Equal(t, "call", traces[1].EthTrace.Type)

	// our transaction will be in the third element of traces with the expected hash
	require.Equal(t, 1, traces[2].TransactionPosition)
	require.Equal(t, hash, traces[2].TransactionHash)
	require.Equal(t, "create", traces[2].EthTrace.Type)

	toBlock = "latest"
	filter = ethtypes.EthTraceFilterCriteria{
		FromBlock:   &fromBlock,
		ToBlock:     &toBlock,
		FromAddress: ethtypes.EthAddressList{ethAddr},
		ToAddress:   ethtypes.EthAddressList{contractAddr},
	}

	// Trace filter should find the transaction
	tracesAddressFilter, err := client.EthTraceFilter(ctx, filter)
	require.NoError(t, err)
	require.NotNil(t, tracesAddressFilter)
	require.NotEmpty(t, tracesAddressFilter)

	// we should only get our contract deploy transaction
	require.Len(t, tracesAddressFilter, 1)
	require.Equal(t, 1, tracesAddressFilter[0].TransactionPosition)
	require.Equal(t, hash, tracesAddressFilter[0].TransactionHash)
	require.Equal(t, "create", tracesAddressFilter[0].EthTrace.Type)

	after := ethtypes.EthUint64(1)
	count := ethtypes.EthUint64(2)
	filter = ethtypes.EthTraceFilterCriteria{
		FromBlock: &fromBlock,
		ToBlock:   &toBlock,
		After:     &after,
		Count:     &count,
	}
	// Trace filter should find the transaction
	tracesAfterCount, err := client.EthTraceFilter(ctx, filter)
	require.NoError(t, err)
	require.NotNil(t, traces)
	require.NotEmpty(t, traces)

	// we should only get the last two results from the first trace query
	require.Len(t, tracesAfterCount, 2)
	require.Equal(t, traces[1].TransactionHash, tracesAfterCount[0].TransactionHash)
	require.Equal(t, traces[2].TransactionHash, tracesAfterCount[1].TransactionHash)

	// make sure we have null rounds in the chain
	bms[0].InjectNulls(2)
	ch, err := client.ChainNotify(ctx)
	require.NoError(t, err)
	hc := <-ch // current
	require.Equal(t, store.HCCurrent, hc[0].Type)
	beforeNullHeight := hc[0].Val.Height()
	var blocks int
	for {
		hc = <-ch // wait for next block
		require.Equal(t, store.HCApply, hc[0].Type)
		if hc[0].Val.Height() > beforeNullHeight {
			blocks++
			if blocks == 2 { // two blocks, so "latest" points to the block after nulls
				break
			}
		}
	}

	// define filter criteria that spans a null round so it has to at lest consider it
	toBlock = "latest"
	filter = ethtypes.EthTraceFilterCriteria{
		FromBlock: &fromBlock,
		ToBlock:   &toBlock,
	}
	traces, err = client.EthTraceFilter(ctx, filter)
	require.NoError(t, err)
	require.Len(t, traces, 3) // still the same traces as before
}
