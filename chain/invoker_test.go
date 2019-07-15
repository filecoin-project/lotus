package chain

import (
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-lotus/chain/types"
)

type basicContract struct{}
type basicParams struct {
	B byte
}

func init() {
	cbor.RegisterCborType(basicParams{})
}

func (b basicContract) Exports() []interface{} {
	return []interface{}{
		b.InvokeSomething0,
		b.BadParam,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		b.InvokeSomething10,
	}
}

func (basicContract) InvokeSomething0(act *types.Actor, vmctx types.VMContext,
	params *basicParams) (types.InvokeRet, error) {
	return types.InvokeRet{
		ReturnCode: params.B,
	}, nil
}
func (basicContract) BadParam(act *types.Actor, vmctx types.VMContext,
	params *basicParams) (types.InvokeRet, error) {
	return types.InvokeRet{
		ReturnCode: 255,
	}, nil
}

func (basicContract) InvokeSomething10(act *types.Actor, vmctx types.VMContext,
	params *basicParams) (types.InvokeRet, error) {
	return types.InvokeRet{
		ReturnCode: params.B + 10,
	}, nil
}

func TestInvokerBasic(t *testing.T) {
	inv := invoker{}
	code, err := inv.transform(basicContract{})
	assert.NoError(t, err)

	{
		bParam, err := cbor.DumpObject(basicParams{B: 1})
		assert.NoError(t, err)

		ret, err := code[0](nil, &VMContext{}, bParam)
		assert.NoError(t, err)
		assert.Equal(t, byte(1), ret.ReturnCode, "return code should be 1")
	}

	{
		bParam, err := cbor.DumpObject(basicParams{B: 2})
		assert.NoError(t, err)

		ret, err := code[10](nil, &VMContext{}, bParam)
		assert.NoError(t, err)
		assert.Equal(t, byte(12), ret.ReturnCode, "return code should be 12")
	}

	_, err = code[1](nil, &VMContext{}, []byte{0})
	assert.Error(t, err)

}
