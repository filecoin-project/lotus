package vm

import (
	"fmt"
	"io"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type basicContract struct{}
type basicParams struct {
	B byte
}

func (b *basicParams) MarshalCBOR(w io.Writer) error {
	_, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(b.B)))
	return err
}

func (b *basicParams) UnmarshalCBOR(r io.Reader) error {
	maj, val, err := cbg.CborReadHeader(r)
	if err != nil {
		return err
	}

	if maj != cbg.MajUnsignedInt {
		return fmt.Errorf("bad cbor type")
	}

	b.B = byte(val)
	return nil
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
		bParam, err := actors.SerializeParams(&basicParams{B: 1})
		assert.NoError(t, err)

		_, aerr := code[0](nil, &VMContext{}, bParam)

		assert.Equal(t, byte(1), aerrors.RetCode(aerr), "return code should be 1")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	{
		bParam, err := actors.SerializeParams(&basicParams{B: 2})
		assert.NoError(t, err)

		_, aerr := code[10](nil, &VMContext{}, bParam)
		assert.Equal(t, byte(12), aerrors.RetCode(aerr), "return code should be 12")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	_, aerr := code[1](nil, &VMContext{}, []byte{99})
	if aerrors.IsFatal(aerr) {
		t.Fatal("err should not be fatal")
	}
	assert.Equal(t, byte(1), aerrors.RetCode(aerr), "return code should be 1")

}
