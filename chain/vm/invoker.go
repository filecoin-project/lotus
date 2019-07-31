package vm

import (
	"fmt"
	"reflect"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
)

type invoker struct {
	builtInCode map[cid.Cid]nativeCode
}

type invokeFunc func(act *types.Actor, vmctx types.VMContext, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc

func newInvoker() *invoker {
	inv := &invoker{
		builtInCode: make(map[cid.Cid]nativeCode),
	}

	// add builtInCode using: register(cid, singleton)
	inv.register(actors.InitActorCodeCid, actors.InitActor{})
	inv.register(actors.StorageMarketActorCodeCid, actors.StorageMarketActor{})
	inv.register(actors.StorageMinerCodeCid, actors.StorageMinerActor{})
	inv.register(actors.MultisigActorCodeCid, actors.MultiSigActor{})

	return inv
}

func (inv *invoker) Invoke(act *types.Actor, vmctx types.VMContext, method uint64, params []byte) ([]byte, aerrors.ActorError) {

	code, ok := inv.builtInCode[act.Code]
	if !ok {
		return nil, aerrors.Newf(255, "no code for actor %s", act.Code)
	}
	if method >= uint64(len(code)) || code[method] == nil {
		return nil, aerrors.Newf(255, "no method %d on actor", method)
	}
	return code[method](act, vmctx, params)

}

func (inv *invoker) register(c cid.Cid, instance Invokee) {
	code, err := inv.transform(instance)
	if err != nil {
		panic(err)
	}
	inv.builtInCode[c] = code
}

type Invokee interface {
	Exports() []interface{}
}

var tVMContext = reflect.TypeOf((*types.VMContext)(nil)).Elem()
var tAError = reflect.TypeOf((*aerrors.ActorError)(nil)).Elem()

func (*invoker) transform(instance Invokee) (nativeCode, error) {
	itype := reflect.TypeOf(instance)
	exports := instance.Exports()
	for i, m := range exports {
		i := i
		newErr := func(format string, args ...interface{}) error {
			str := fmt.Sprintf(format, args...)
			return fmt.Errorf("transform(%s) export(%d): %s", itype.Name(), i, str)
		}
		if m == nil {
			continue
		}
		meth := reflect.ValueOf(m)
		t := meth.Type()
		if t.Kind() != reflect.Func {
			return nil, newErr("is not a function")
		}
		if t.NumIn() != 3 {
			return nil, newErr("wrong number of inputs should be: " +
				"*types.Actor, *VMContext, <type of parameter>")
		}
		if t.In(0) != reflect.TypeOf(&types.Actor{}) {
			return nil, newErr("first arguemnt should be *types.Actor")
		}
		if t.In(1) != tVMContext {
			return nil, newErr("second argument should be types.VMContext")
		}

		if t.In(2).Kind() != reflect.Ptr {
			return nil, newErr("parameter has to be a pointer to parameter, is: %s",
				t.In(2).Kind())
		}

		if t.NumOut() != 2 {
			return nil, newErr("wrong number of outputs should be: " +
				"(InvokeRet, error)")
		}
		if t.Out(0) != reflect.TypeOf([]byte{}) {
			return nil, newErr("first output should be slice of bytes")
		}
		if !t.Out(1).Implements(tAError) {
			return nil, newErr("second output should be ActorError type")
		}

	}
	code := make(nativeCode, len(exports))
	for id, m := range exports {
		meth := reflect.ValueOf(m)
		code[id] = reflect.MakeFunc(reflect.TypeOf((invokeFunc)(nil)),
			func(in []reflect.Value) []reflect.Value {
				paramT := meth.Type().In(2).Elem()
				param := reflect.New(paramT)

				inBytes := in[2].Interface().([]byte)
				err := cbor.DecodeInto(inBytes, param.Interface())
				if err != nil {
					aerr := aerrors.Absorb(err, 1, "failed to decode parameters")
					return []reflect.Value{
						reflect.ValueOf([]byte{}),
						// Below is a hack, fixed in Go 1.13
						// https://git.io/fjXU6
						reflect.ValueOf(&aerr).Elem(),
					}
				}

				return meth.Call([]reflect.Value{
					in[0], in[1], param,
				})
			}).Interface().(invokeFunc)

	}
	return code, nil
}
