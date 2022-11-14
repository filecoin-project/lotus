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
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestFEVMBasic does a basic fevm contract installation and invocation
func TestFEVMBasic(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
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

	nonce, err := client.MpoolGetNonce(ctx, fromAddr)
	if err != nil {
		nonce = 0 // assume a zero nonce on error (e.g. sender doesn't exist).
	}

	var salt [32]byte
	binary.BigEndian.PutUint64(salt[:], nonce)

	method := builtintypes.MethodsEAM.Create2
	params, err := actors.SerializeParams(&eam.Create2Params{
		Initcode: contract,
		Salt:     salt,
	})
	require.NoError(t, err)

	msg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   fromAddr,
		Value:  big.Zero(),
		Method: method,
		Params: params,
	}

	t.Log("sending create message")
	smsg, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	t.Log("waiting for message to execute")
	wait, err := client.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(t, err)

	require.Equal(t, int(wait.Receipt.ExitCode), 0, "contract installation failed")

	var result eam.CreateReturn
	r := bytes.NewReader(wait.Receipt.Return)
	err = result.UnmarshalCBOR(r)
	require.NoError(t, err)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(t, err)
	t.Logf("actor ID address is %s", idAddr)

	// invoke the contract with owner
	{
		entryPoint, err := hex.DecodeString("f8b2cb4f")
		require.NoError(t, err)

		inputData, err := hex.DecodeString("000000000000000000000000ff00000000000000000000000000000000000064")
		require.NoError(t, err)

		params := append(entryPoint, inputData...)
		var buffer bytes.Buffer
		err = cbg.WriteByteArray(&buffer, params)
		require.NoError(t, err)
		params = buffer.Bytes()

		msg := &types.Message{
			To:     idAddr,
			From:   fromAddr,
			Value:  big.Zero(),
			Method: abi.MethodNum(2),
			Params: params,
		}

		t.Log("sending invoke message")
		smsg, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		t.Log("waiting for message to execute")
		wait, err := client.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
		require.NoError(t, err)

		require.Equal(t, int(wait.Receipt.ExitCode), 0, "contract execution failed")

		result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
		require.NoError(t, err)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000002710")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)
	}

	// invoke the contract with non owner
	{
		entryPoint, err := hex.DecodeString("f8b2cb4f")
		require.NoError(t, err)

		inputData, err := hex.DecodeString("000000000000000000000000ff00000000000000000000000000000000000065")
		require.NoError(t, err)

		params := append(entryPoint, inputData...)
		var buffer bytes.Buffer
		err = cbg.WriteByteArray(&buffer, params)
		require.NoError(t, err)
		params = buffer.Bytes()

		msg := &types.Message{
			To:     idAddr,
			From:   fromAddr,
			Value:  big.Zero(),
			Method: abi.MethodNum(2),
			Params: params,
		}

		t.Log("sending invoke message")
		smsg, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		t.Log("waiting for message to execute")
		wait, err := client.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
		require.NoError(t, err)

		require.Equal(t, int(wait.Receipt.ExitCode), 0, "contract execution failed")

		result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
		require.NoError(t, err)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)

	}
}
