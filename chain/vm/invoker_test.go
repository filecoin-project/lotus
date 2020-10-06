package vm

import (
	"fmt"
	"io"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/specs-actors/actors/runtime"
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

func (basicContract) InvokeSomething0(rt runtime.Runtime, params *basicParams) *abi.EmptyValue {
	rt.Abortf(exitcode.ExitCode(params.B), "params.B")
	return nil
}

func (basicContract) BadParam(rt runtime.Runtime, params *basicParams) *abi.EmptyValue {
	rt.Abortf(255, "bad params")
	return nil
}

func (basicContract) InvokeSomething10(rt runtime.Runtime, params *basicParams) *abi.EmptyValue {
	rt.Abortf(exitcode.ExitCode(params.B+10), "params.B")
	return nil
}

func TestInvokerBasic(t *testing.T) {
	inv := ActorRegistry{}
	code, err := inv.transform(basicContract{})
	assert.NoError(t, err)

	{
		bParam, err := actors.SerializeParams(&basicParams{B: 1})
		assert.NoError(t, err)

		_, aerr := code[0](&Runtime{}, bParam)

		assert.Equal(t, exitcode.ExitCode(1), aerrors.RetCode(aerr), "return code should be 1")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	{
		bParam, err := actors.SerializeParams(&basicParams{B: 2})
		assert.NoError(t, err)

		_, aerr := code[10](&Runtime{}, bParam)
		assert.Equal(t, exitcode.ExitCode(12), aerrors.RetCode(aerr), "return code should be 12")
		if aerrors.IsFatal(aerr) {
			t.Fatal("err should not be fatal")
		}
	}

	_, aerr := code[1](&Runtime{}, []byte{99})
	if aerrors.IsFatal(aerr) {
		t.Fatal("err should not be fatal")
	}
	assert.Equal(t, exitcode.ExitCode(1), aerrors.RetCode(aerr), "return code should be 1")

}
