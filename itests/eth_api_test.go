package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	inittypes "github.com/filecoin-project/go-state-types/builtin/v8/init"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/result"
)

func TestETHGetBlockByHashWithCache(t *testing.T) {
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

	// Submit transaction with valid signature
	client.EVM().SignTransaction(&tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, &tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	blkHash := receipt.BlockHash

	// call EthGetBlockByHash without tx info
	blk1, err := client.EthGetBlockByHash(ctx, blkHash, false)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk1.Hash)

	// call again to exercise the cache
	blk2, err := client.EthGetBlockByHash(ctx, blkHash, false)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk2.Hash)

	// call EthGetBlockByHash with tx info
	blk3, err := client.EthGetBlockByHash(ctx, blkHash, true)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk3.Hash)

	// call again to exercise the cache
	blk4, err := client.EthGetBlockByHash(ctx, blkHash, true)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk4.Hash)
}

func TestETHGetBlockByHashWithoutCache(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableETHBlockCache())

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

	// Submit transaction with valid signature
	client.EVM().SignTransaction(&tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, &tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	blkHash := receipt.BlockHash

	// call EthGetBlockByHash without tx info
	blk1, err := client.EthGetBlockByHash(ctx, blkHash, false)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk1.Hash)

	// call again
	blk2, err := client.EthGetBlockByHash(ctx, blkHash, false)
	require.NoError(t, err)
	require.EqualValues(t, blkHash, blk2.Hash)
}

func TestEthAddressToFilecoinAddress(t *testing.T) {
	// Disable EthRPC to confirm that this method does NOT need the EthEnableRPC config set to true
	client, _, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpKey, err := key.GenerateKey(types.KTDelegated)
	require.NoError(t, err)

	filecoinKeyAddr, err := client.WalletImport(ctx, &secpKey.KeyInfo)
	require.NoError(t, err)

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(filecoinKeyAddr)
	require.NoError(t, err)

	apiFilAddr, err := client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(t, err)

	require.Equal(t, filecoinKeyAddr, apiFilAddr)

	filecoinIdArr := builtin.StorageMarketActorAddr
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(filecoinIdArr)
	require.NoError(t, err)

	apiFilAddr, err = client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(t, err)

	require.Equal(t, filecoinIdArr, apiFilAddr)

}

func TestFilecoinAddressToEthAddress(t *testing.T) {
	require := require.New(t)
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Test for f4 address
	filecoinDelegatedAddr, err := client.WalletNew(ctx, types.KTDelegated)
	require.NoError(err)
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(filecoinDelegatedAddr)
	require.NoError(err)

	apiEthAddr, err := client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinDelegatedAddr}),
	).Assert(require.NoError))

	require.NoError(err)
	require.Equal(ethAddr, apiEthAddr)
	fAddr, err := client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(err)
	require.Equal(filecoinDelegatedAddr, fAddr)

	// test for f0 address
	filecoinIdArr := builtin.StorageMarketActorAddr
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(filecoinIdArr)
	require.NoError(err)

	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinIdArr}),
	).Assert(require.NoError))
	require.NoError(err)

	require.Equal(ethAddr, apiEthAddr)
	fAddr, err = client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(err)
	require.Equal(filecoinIdArr, fAddr)

	// test for f1 address that does not yet exist on chain -> fails
	filecoinSecpAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(err)

	_, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinSecpAddr, "latest"}),
	).Assert(require.NoError))
	require.Error(err)
	require.ErrorContains(err, "actor not found")

	// test for f1 address that exists on chain by sending funds to the above f1 Actor -> works
	kit.SendFunds(ctx, t, client, filecoinSecpAddr, abi.NewTokenAmount(1))
	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinSecpAddr, "latest"}),
	).Assert(require.NoError))
	require.NoError(err)

	idAddr, err := client.StateLookupID(ctx, filecoinSecpAddr, types.EmptyTSK)
	require.NoError(err)
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(err)
	require.Equal(ethAddr, apiEthAddr)
	fAddr, err = client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(err)
	require.Equal(idAddr, fAddr)

	// test for f2 address that exists on chain -> works
	signer := client.DefaultKey.Address

	// Create the multisig
	cp, err := client.MsigCreate(ctx, 1, []address.Address{signer}, 0, types.NewInt(1), signer, big.Zero())
	require.NoError(err, "failed to create multisig (MsigCreate)")

	cm, err := client.MpoolPushMessage(ctx, &cp.Message, nil)
	require.NoError(err, "failed to create multisig (MpooPushMessage)")

	ml, err := client.StateWaitMsg(ctx, cm.Cid(), 5, 100, false)
	require.NoError(err, "failed to create multisig (StateWaitMsg)")
	require.Equal(ml.Receipt.ExitCode, exitcode.Ok)

	var execreturn inittypes.ExecReturn
	err = execreturn.UnmarshalCBOR(bytes.NewReader(ml.Receipt.Return))
	require.NoError(err, "failed to decode multisig create return")

	multisigAddress := execreturn.RobustAddress
	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{multisigAddress, "latest"}),
	).Assert(require.NoError))
	require.NoError(err)

	idAddr, err = client.StateLookupID(ctx, multisigAddress, types.EmptyTSK)
	require.NoError(err)

	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(err)
	require.Equal(ethAddr, apiEthAddr)
}

func TestFilecoinAddressToEthAddressFinalised(t *testing.T) {
	require := require.New(t)
	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// wait till we have enough epochs to perform a finality lookback
	_ = client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality))

	filecoinSecpAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(err)
	kit.SendFunds(ctx, t, client, filecoinSecpAddr, abi.NewTokenAmount(1))
	// works for latest
	_, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinSecpAddr, "latest"}),
	).Assert(require.NoError))
	require.NoError(err)

	// but fails for finalised
	_, err = client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinSecpAddr, "finalized"}),
	).Assert(require.NoError))
	require.Error(err)
	require.Contains(err.Error(), "sufficient epochs have passed")

	// wait for enough epochs to pass for finalised to work
	ts, err := client.ChainHead(ctx)
	require.NoError(err)
	_ = client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality+ts.Height()+5))
	// finalized should now work
	apiEthAddr, err := client.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{filecoinSecpAddr, "finalized"}),
	).Assert(require.NoError))
	require.NoError(err)

	idAddr, err := client.StateLookupID(ctx, filecoinSecpAddr, types.EmptyTSK)
	require.NoError(err)
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(err)
	require.Equal(ethAddr, apiEthAddr)
}

func TestEthGetGenesis(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ethBlk, err := client.EVM().EthGetBlockByNumber(ctx, "0x0", true)
	require.NoError(t, err)

	genesis, err := client.ChainGetGenesis(ctx)
	require.NoError(t, err)

	genesisCid, err := genesis.Key().Cid()
	require.NoError(t, err)

	genesisHash, err := ethtypes.EthHashFromCid(genesisCid)
	require.NoError(t, err)
	require.Equal(t, ethBlk.Hash, genesisHash)
}

func TestNetVersion(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	version, err := client.NetVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(buildconstants.Eip155ChainId), version)
}

func TestEthBlockNumberAliases(t *testing.T) {
	blockTime := 2 * time.Millisecond
	kit.QuietMiningLogs()
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	ens.Start()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality+100))

	for _, tc := range []struct {
		param       string
		expectedLag abi.ChainEpoch
	}{
		{"latest", 1},                       // head - 1
		{"safe", 200},                       // "latest" - 200 when F3 isn't running
		{"finalized", policy.ChainFinality}, // "latest" - 900
	} {
		t.Run(tc.param, func(t *testing.T) {
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)
			var blk ethtypes.EthBlock
			for { // get a block while retaining a stable "head" reference
				blk, err = client.EVM().EthGetBlockByNumber(ctx, tc.param, true)
				require.NoError(t, err)
				afterHead, err := client.ChainHead(ctx)
				require.NoError(t, err)
				if afterHead.Height() == head.Height() {
					break
				}
				// else: whoops, we had a chain increment between getting head and getting "latest" so
				// we won't be able to use head as a stable reference for comparison
				head = afterHead
			}
			ts, err := client.ChainGetTipSetByHeight(ctx, head.Height()-tc.expectedLag, head.Key())
			require.NoError(t, err)
			require.EqualValues(t, ts.Height(), blk.Number)
		})
	}
}
