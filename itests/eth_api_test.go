package itests

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/build"
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
	// Disable EthRPC to confirm that this method does NOT need the EthEnableRPC config set to true
	client, _, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secpDelegatedKey, err := key.GenerateKey(types.KTDelegated)
	require.NoError(t, err)

	filecoinKeyAddr, err := client.WalletImport(ctx, &secpDelegatedKey.KeyInfo)
	require.NoError(t, err)

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(filecoinKeyAddr)
	require.NoError(t, err)

	apiEthAddr, err := client.FilecoinAddressToEthAddress(ctx, filecoinKeyAddr)
	require.NoError(t, err)

	require.Equal(t, ethAddr, apiEthAddr)

	filecoinIdArr := builtin.StorageMarketActorAddr
	ethAddr, err = ethtypes.EthAddressFromFilecoinAddress(filecoinIdArr)
	require.NoError(t, err)

	apiEthAddr, err = client.FilecoinAddressToEthAddress(ctx, filecoinIdArr)
	require.NoError(t, err)

	require.Equal(t, ethAddr, apiEthAddr)

	secpKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	filecoinSecpAddr, err := client.WalletImport(ctx, &secpKey.KeyInfo)
	require.NoError(t, err)

	_, err = client.FilecoinAddressToEthAddress(ctx, filecoinSecpAddr)

	require.ErrorContains(t, err, ethtypes.ErrInvalidAddress.Error())
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
	require.Equal(t, strconv.Itoa(build.Eip155ChainId), version)
}
