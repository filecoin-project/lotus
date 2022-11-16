package kit

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"

	"github.com/filecoin-project/lotus/api"
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

func (e *EVM) InvokeSolidity(ctx context.Context, sender address.Address, target address.Address, selector []byte, inputData []byte) *api.MsgLookup {
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

// LoadEvents loads all events in an event AMT.
func (e *EVM) LoadEvents(ctx context.Context, eventsRoot cid.Cid) []types.Event {
	require := require.New(e.t)

	s := &apiIpldStore{ctx, e}
	amt, err := amt4.LoadAMT(ctx, s, eventsRoot, amt4.UseTreeBitWidth(5))
	require.NoError(err)

	ret := make([]types.Event, 0, amt.Len())
	err = amt.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
		var evt types.Event
		if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
			return err
		}
		ret = append(ret, evt)
		return nil
	})
	require.NoError(err)
	return ret
}

// TODO: cleanup and put somewhere reusable.
type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("object does not implement CBORUnmarshaler")
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	panic("No mutations allowed")
}
