package chain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-lotus/chain/types"
)

type basicContract struct{}
type basicParams struct {
	b byte
}

func (b *basicParams) UnmarshalCBOR(in []byte) (int, error) {
	b.b = in[0]
	return 1, nil
}

type badParam struct {
}

func (b *badParam) UnmarshalCBOR(in []byte) (int, error) {
	return -1, errors.New("some error")
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
	params *basicParams) (InvokeRet, error) {
	return InvokeRet{
		returnCode: params.b,
	}, nil
}
func (basicContract) BadParam(act *types.Actor, vmctx types.VMContext,
	params *badParam) (InvokeRet, error) {
	panic("should not execute")
}

func (basicContract) InvokeSomething10(act *types.Actor, vmctx types.VMContext,
	params *basicParams) (InvokeRet, error) {
	return InvokeRet{
		returnCode: params.b + 10,
	}, nil
}

func TestInvokerBasic(t *testing.T) {
	inv := invoker{}
	code, err := inv.transform(basicContract{})
	assert.NoError(t, err)
	ret, err := code[0](nil, nil, []byte{1})
	assert.NoError(t, err)
	assert.Equal(t, byte(1), ret.returnCode, "return code should be 1")

	ret, err = code[10](nil, &VMContext{}, []byte{2})
	assert.NoError(t, err)
	assert.Equal(t, byte(12), ret.returnCode, "return code should be 1")

	ret, err = code[1](nil, &VMContext{}, []byte{2})
	assert.Error(t, err)
}
