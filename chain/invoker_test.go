package chain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type basicContract struct{}
type basicParams struct {
	b byte
}

func (b *basicParams) UnmarshalCBOR(in []byte) (int, error) {
	b.b = in[0]
	return 1, nil
}

func (_ basicContract) InvokeSomething0(act *Actor, vmctx *VMContext,
	params *basicParams) (InvokeRet, error) {
	return InvokeRet{
		returnCode: params.b,
	}, nil
}
func (_ basicContract) InvokeSomething10(act *Actor, vmctx *VMContext,
	params *basicParams) (InvokeRet, error) {
	return InvokeRet{
		returnCode: params.b,
	}, nil
}

func TestInvokerBasic(t *testing.T) {
	inv := invoker{}
	code, err := inv.transform(basicContract{})
	assert.NoError(t, err)
	ret, err := code[0](nil, nil, []byte{1})
	assert.NoError(t, err)
	assert.Equal(t, byte(1), ret.returnCode, "return code should be 1")

	ret, err = code[10](nil, nil, []byte{1})
	assert.NoError(t, err)
	assert.Equal(t, byte(1), ret.returnCode, "return code should be 1")
}
