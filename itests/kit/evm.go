package kit

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

// EVM groups EVM-related actions.
type EVM struct{ *TestFullNode }

func (f *TestFullNode) EVM() *EVM {
	return &EVM{f}
}

func (e *EVM) DeployContract(ctx context.Context, sender address.Address, bytecode []byte) eam.CreateReturn {
	require := require.New(e.t)

	nonce, err := e.MpoolGetNonce(ctx, sender)
	if err != nil {
		nonce = 0 // assume a zero nonce on error (e.g. sender doesn't exist).
	}

	var salt [32]byte
	binary.BigEndian.PutUint64(salt[:], nonce)

	method := builtintypes.MethodsEAM.Create2
	params, err := actors.SerializeParams(&eam.Create2Params{
		Initcode: bytecode,
		Salt:     salt,
	})
	require.NoError(err)

	msg := &types.Message{
		To:     builtintypes.EthereumAddressManagerActorAddr,
		From:   sender,
		Value:  big.Zero(),
		Method: method,
		Params: params,
	}

	e.t.Log("sending create message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	require.NoError(err)

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(err)

	require.True(wait.Receipt.ExitCode.IsSuccess(), "contract installation failed")

	var result eam.CreateReturn
	r := bytes.NewReader(wait.Receipt.Return)
	err = result.UnmarshalCBOR(r)
	require.NoError(err)

	return result
}

func (e *EVM) InvokeSolidity(ctx context.Context, sender address.Address, target address.Address, selector []byte, inputData []byte) *types.MsgLookup {
	require := require.New(e.t)

	params := append(selector, inputData...)
	var buffer bytes.Buffer
	err := cbg.WriteByteArray(&buffer, params)
	require.NoError(err)
	params = buffer.Bytes()

	msg := &types.Message{
		To:     target,
		From:   sender,
		Value:  big.Zero(),
		Method: abi.MethodNum(2),
		Params: params,
	}

	e.t.Log("sending invoke message")
	smsg, err := e.MpoolPushMessage(ctx, msg, nil)
	require.NoError(err)

	e.t.Log("waiting for message to execute")
	wait, err := e.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
	require.NoError(err)

	return wait
}
