package chain

import (
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
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
	params *basicParams) ([]byte, aerrors.ActorError) {
	return nil, aerrors.New(params.B, "params.B")
}
func (basicContract) BadParam(act *types.Actor, vmctx types.VMContext,
	params *basicParams) ([]byte, aerrors.ActorError) {
	return nil, aerrors.New(255, "bad params")
}

func (basicContract) InvokeSomething10(act *types.Actor, vmctx types.VMContext,
	params *basicParams) ([]byte, aerrors.ActorError) {
	return nil, aerrors.New(params.B+10, "params.B")
}

func TestInvokerBasic(t *testing.T) {
	inv := invoker{}
	code, err := inv.transform(basicContract{})
	assert.NoError(t, err)

	{
		bParam, err := cbor.DumpObject(basicParams{B: 1})
		assert.NoError(t, err)

		_, aerr := code[0](nil, &VMContext{}, bParam)

		assert.Equal(t, byte(1), aerrors.RetCode(aerr), "return code should be 1")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	{
		bParam, err := cbor.DumpObject(basicParams{B: 2})
		assert.NoError(t, err)

		_, aerr := code[10](nil, &VMContext{}, bParam)
		assert.Equal(t, byte(12), aerrors.RetCode(aerr), "return code should be 12")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	_, aerr := code[1](nil, &VMContext{}, []byte{0})
	if aerrors.IsFatal(aerr) {
		t.Fatal("err should not be fatal")
	}
	assert.Equal(t, byte(1), aerrors.RetCode(aerr), "return code should be 1")

}
