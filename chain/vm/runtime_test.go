package vm

import (
	"io"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
)

type NotAVeryGoodMarshaler struct{}

func (*NotAVeryGoodMarshaler) MarshalCBOR(writer io.Writer) error {
	return xerrors.Errorf("no")
}

var _ cbg.CBORMarshaler = &NotAVeryGoodMarshaler{}

func TestRuntimePutErrors(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Fatal("expected non-nil recovery")
		}

		aerr := err.(aerrors.ActorError)
		if aerr.IsFatal() {
			t.Fatal("expected non-fatal actor error")
		}

		if aerr.RetCode() != exitcode.ErrSerialization {
			t.Fatal("expected serialization error")
		}
	}()

	rt := Runtime{
		cst: cbor.NewCborStore(nil),
	}

	rt.StorePut(&NotAVeryGoodMarshaler{})
	t.Error("expected panic")
}

func BenchmarkRuntime_CreateRuntimeChargeGas_TracingDisabled(b *testing.B) {
	var (
		cst = cbor.NewCborStore(nil)
		gch = newGasCharge("foo", 1000, 1000)
	)

	b.ResetTimer()

	EnableDetailedTracing = false
	noop := func() bool { return EnableDetailedTracing }
	for n := 0; n < b.N; n++ {
		// flip the value and access it to make sure
		// the compiler doesn't optimize away
		EnableDetailedTracing = true
		_ = noop()
		EnableDetailedTracing = false
		_ = (&Runtime{cst: cst}).chargeGasInternal(gch, 0)
	}
}
