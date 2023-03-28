package itests

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestEthGetBalanceExistingF4address(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, ethAddr, deployer := client.EVM().NewAccount()

	fundAmount := types.FromFil(0)
	// send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, fundAmount)

	balance, err := client.EthGetBalance(ctx, ethAddr, "latest")
	require.NoError(t, err)
	require.Equal(t, balance, ethtypes.EthBigInt{Int: fundAmount.Int})
}

func TestEthGetBalanceNonExistentF4address(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, ethAddr, _ := client.EVM().NewAccount()

	balance, err := client.EthGetBalance(ctx, ethAddr, "latest")
	require.NoError(t, err)
	require.Equal(t, balance, ethtypes.EthBigIntZero)
}

func TestEthGetBalanceExistentIDMaskedAddr(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	faddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)
	fid, err := client.StateLookupID(ctx, faddr, types.EmptyTSK)
	require.NoError(t, err)

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(fid)
	require.NoError(t, err)

	balance, err := client.WalletBalance(ctx, fid)
	require.NoError(t, err)

	ebal, err := client.EthGetBalance(ctx, ethAddr, "latest")
	require.NoError(t, err)
	require.Equal(t, ebal, ethtypes.EthBigInt{Int: balance.Int})
}

func TestEthGetBalanceBuiltinActor(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Address for market actor
	fid, err := address.NewFromString("f05")
	require.NoError(t, err)

	kit.SendFunds(ctx, t, client, fid, abi.TokenAmount{Int: big.NewInt(10).Int})

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(fid)
	require.NoError(t, err)

	ebal, err := client.EthGetBalance(ctx, ethAddr, "latest")
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBigInt{Int: big.NewInt(10).Int}, ebal)
}

func TestEthBalanceCorrectLookup(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, ethAddr, filAddr := client.EVM().NewAccount()

	val := int64(100)

	smsg, err := client.MpoolPushMessage(ctx, &types.Message{
		To:    filAddr,
		From:  client.DefaultKey.Address,
		Value: abi.NewTokenAmount(val),
	}, nil)
	require.NoError(t, err)

	ml, err := client.StateWaitMsg(ctx, smsg.Cid(), 3, api.LookbackNoLimit, false)
	require.NoError(t, err)
	require.True(t, ml.Receipt.ExitCode.IsSuccess())

	bal, err := client.EVM().EthGetBalance(ctx, ethAddr, strconv.FormatInt(int64(ml.Height-2), 10))
	require.NoError(t, err)
	require.Equal(t, int64(0), bal.Int64())

	bal, err = client.EVM().EthGetBalance(ctx, ethAddr, strconv.FormatInt(int64(ml.Height-1), 10))
	require.NoError(t, err)
	require.Equal(t, val, bal.Int64())

	bal, err = client.EVM().EthGetBalance(ctx, ethAddr, strconv.FormatInt(int64(ml.Height), 10))
	require.NoError(t, err)
	require.Equal(t, val, bal.Int64())
}
