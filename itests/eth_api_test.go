package itests

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

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
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mkParams := func(addr address.Address, blkNum string) jsonrpc.RawParams {
		fp := new(ethtypes.FilecoinAddressToEthAddressParams)
		fp.FilecoinAddress = addr

		if blkNum != "" {
			fp.BlkParam = &blkNum
		}

		params, err := json.Marshal(fp)
		require.NoError(t, err)

		return jsonrpc.RawParams(params)
	}

	// Test for f4 address
	filecoinDelegatedAddr, err := client.WalletNew(ctx, types.KTDelegated)
	require.NoError(t, err)
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(filecoinDelegatedAddr)
	require.NoError(t, err)
	apiEthAddr, err := client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinDelegatedAddr, ""))
	require.NoError(t, err)
	require.Equal(t, ethAddr, apiEthAddr)
	fAddr, err := client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(t, err)
	require.Equal(t, filecoinDelegatedAddr, fAddr)

	// test for f0 address
	filecoinIdArr := builtin.StorageMarketActorAddr
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(filecoinIdArr)
	require.NoError(t, err)
	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinIdArr, ""))
	require.NoError(t, err)
	require.Equal(t, ethAddr, apiEthAddr)
	fAddr, err = client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(t, err)
	require.Equal(t, filecoinIdArr, fAddr)

	// test for f1 address that does not yet exist on chain -> fails
	filecoinSecpAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)
	_, err = client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinSecpAddr, "latest"))
	require.ErrorContains(t, err, "actor not found")

	// test for f1 address that exists on chain by sending funds to the above f1 Actor -> works
	kit.SendFunds(ctx, t, client, filecoinSecpAddr, abi.NewTokenAmount(1))
	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinSecpAddr, "latest"))
	require.NoError(t, err)
	idAddr, err := client.StateLookupID(ctx, filecoinSecpAddr, types.EmptyTSK)
	require.NoError(t, err)
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(t, err)
	require.Equal(t, ethAddr, apiEthAddr)
	fAddr, err = client.EthAddressToFilecoinAddress(ctx, ethAddr)
	require.NoError(t, err)
	require.Equal(t, idAddr, fAddr)

	// test for f2 address that exists on chain -> works
	signer := client.DefaultKey.Address

	// Create the multisig
	cp, err := client.MsigCreate(ctx, 1, []address.Address{signer}, 0, types.NewInt(1), signer, big.Zero())
	require.NoError(t, err, "failed to create multisig (MsigCreate)")

	cm, err := client.MpoolPushMessage(ctx, &cp.Message, nil)
	require.NoError(t, err, "failed to create multisig (MpooPushMessage)")

	ml, err := client.StateWaitMsg(ctx, cm.Cid(), 5, 100, false)
	require.NoError(t, err, "failed to create multisig (StateWaitMsg)")
	require.Equal(t, ml.Receipt.ExitCode, exitcode.Ok)

	var execreturn inittypes.ExecReturn
	err = execreturn.UnmarshalCBOR(bytes.NewReader(ml.Receipt.Return))
	require.NoError(t, err, "failed to decode multisig create return")

	multisigAddress := execreturn.RobustAddress
	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, mkParams(multisigAddress, "latest"))
	require.NoError(t, err)

	idAddr, err = client.StateLookupID(ctx, multisigAddress, types.EmptyTSK)
	require.NoError(t, err)

	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(t, err)
	require.Equal(t, ethAddr, apiEthAddr)
}

func TestFilecoinAddressToEthAddressFinalised(t *testing.T) {
	mkParams := func(addr address.Address, blkNum string) jsonrpc.RawParams {
		fp := new(ethtypes.FilecoinAddressToEthAddressParams)
		fp.FilecoinAddress = addr

		if blkNum != "" {
			fp.BlkParam = &blkNum
		}

		params, err := json.Marshal(fp)
		require.NoError(t, err)

		return jsonrpc.RawParams(params)
	}

	blockTime := 5 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// wait till we have enough epochs to perform a finality lookback
	_ = client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality))

	filecoinSecpAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)
	kit.SendFunds(ctx, t, client, filecoinSecpAddr, abi.NewTokenAmount(1))
	// works for latest
	_, err = client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinSecpAddr, "latest"))
	require.NoError(t, err)
	// but fails for finalised
	_, err = client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinSecpAddr, "finalized"))
	require.Contains(t, err.Error(), "sufficient epochs have passed")

	// wait for enough epochs to pass for finalised to work
	ts, err := client.ChainHead(ctx)
	require.NoError(t, err)
	_ = client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality+ts.Height()+5))
	// finalized should now work
	apiEthAddr, err := client.FilecoinAddressToEthAddress(ctx, mkParams(filecoinSecpAddr, "finalized"))
	require.NoError(t, err)

	idAddr, err := client.StateLookupID(ctx, filecoinSecpAddr, types.EmptyTSK)
	require.NoError(t, err)
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(idAddr)
	require.NoError(t, err)
	require.Equal(t, ethAddr, apiEthAddr)
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

	build.Clock.Sleep(time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	head := client.WaitTillChain(ctx, kit.HeightAtLeast(policy.ChainFinality+100))

	// latest should be head-1 (parents)
	latestEthBlk, err := client.EVM().EthGetBlockByNumber(ctx, "latest", true)
	require.NoError(t, err)
	diff := int64(latestEthBlk.Number) - int64(head.Height()-1)
	require.GreaterOrEqual(t, diff, int64(0))
	require.LessOrEqual(t, diff, int64(2))

	// safe should be latest-30
	safeEthBlk, err := client.EVM().EthGetBlockByNumber(ctx, "safe", true)
	require.NoError(t, err)
	diff = int64(latestEthBlk.Number-30) - int64(safeEthBlk.Number)
	require.GreaterOrEqual(t, diff, int64(0))
	require.LessOrEqual(t, diff, int64(2))

	// finalized should be Finality blocks behind latest
	finalityEthBlk, err := client.EVM().EthGetBlockByNumber(ctx, "finalized", true)
	require.NoError(t, err)
	diff = int64(latestEthBlk.Number) - int64(policy.ChainFinality) - int64(finalityEthBlk.Number)
	require.GreaterOrEqual(t, diff, int64(0))
	require.LessOrEqual(t, diff, int64(2))
}
