package itests

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

func TestAddressCreationBeforeDeploy(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/SimpleCoin.bin")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)
	fromId, err := client.StateLookupID(ctx, fromAddr, types.EmptyTSK)
	require.NoError(t, err)

	senderEthAddr, err := ethtypes.NewEthAddressFromFilecoinAddress(fromId)
	require.NoError(t, err)

	var salt [32]byte
	binary.BigEndian.PutUint64(salt[:], 1)

	// Generate contract address before actually deploying contract
	ethAddr, err := ethtypes.GetContractEthAddressFromCode(senderEthAddr, salt, contract)
	require.NoError(t, err)

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

	// Check if actor at new address is an embryo actor
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, builtin.IsEmbryoActor(actor.Code))

	// Create and deploy evm actor

	method := builtintypes.MethodsEAM.Create2
	params, err := actors.SerializeParams(&eam.Create2Params{
		Initcode: contract,
		Salt:     salt,
	})
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

	// Check if eth address returned from Create2 is the same as eth address predicted at the start
	var create2Return eam.Create2Return
	err = create2Return.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return))
	require.NoError(t, err)

	createdEthAddr, err := ethtypes.NewEthAddressFromBytes(create2Return.EthAddress[:])
	require.NoError(t, err)
	require.Equal(t, ethAddr, createdEthAddr)

	// Check if newly deployed actor still has funds
	actorPostCreate, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, actorPostCreate.Balance, sendAmount)
	require.True(t, builtin.IsEvmActor(actorPostCreate.Code))

}
