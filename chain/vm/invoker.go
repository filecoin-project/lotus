package vm

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type invoker struct {
	builtInCode  map[cid.Cid]nativeCode
	builtInState map[cid.Cid]reflect.Type
}

type invokeFunc func(act *types.Actor, vmctx types.VMContext, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc

func newInvoker() *invoker {
	inv := &invoker{
		builtInCode:  make(map[cid.Cid]nativeCode),
		builtInState: make(map[cid.Cid]reflect.Type),
	}

	// add builtInCode using: register(cid, singleton)
	inv.register(actors.InitCodeCid, actors.InitActor{}, actors.InitActorState{})
	inv.register(actors.CronCodeCid, actors.CronActor{}, actors.CronActorState{})
	inv.register(actors.StoragePowerCodeCid, actors.StoragePowerActor{}, actors.StoragePowerState{})
	inv.register(actors.StorageMarketCodeCid, actors.StorageMarketActor{}, actors.StorageMarketState{})
	inv.register(actors.StorageMinerCodeCid, actors.StorageMinerActor{}, actors.StorageMinerActorState{})
	inv.register(actors.StorageMiner2CodeCid, actors.StorageMinerActor2{}, actors.StorageMinerActorState{})
	inv.register(actors.MultisigCodeCid, actors.MultiSigActor{}, actors.MultiSigActorState{})
	inv.register(actors.PaymentChannelCodeCid, actors.PaymentChannelActor{}, actors.PaymentChannelActorState{})

	return inv
}

func (inv *invoker) Invoke(act *types.Actor, vmctx types.VMContext, method uint64, params []byte) ([]byte, aerrors.ActorError) {

	if act.Code == actors.AccountCodeCid {
		return nil, aerrors.Newf(254, "cannot invoke methods on account actors")
	}

	code, ok := inv.builtInCode[act.Code]
	if !ok {
		log.Errorf("no code for actor %s (Addr: %s)", act.Code, vmctx.Message().To)
		return nil, aerrors.Newf(255, "no code for actor %s(%d)(%s)", act.Code, method, hex.EncodeToString(params))
	}
	if method >= uint64(len(code)) || code[method] == nil {
		return nil, aerrors.Newf(255, "no method %d on actor", method)
	}
	return code[method](act, vmctx, params)

}

func (inv *invoker) register(c cid.Cid, instance Invokee, state interface{}) {
	code, err := inv.transform(instance)
	if err != nil {
		panic(err)
	}
	inv.builtInCode[c] = code
	inv.builtInState[c] = reflect.TypeOf(state)
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
				if len(inBytes) > 0 {
					if err := DecodeParams(inBytes, param.Interface()); err != nil {
						aerr := aerrors.Absorb(err, 1, "failed to decode parameters")
						return []reflect.Value{
							reflect.ValueOf([]byte{}),
							// Below is a hack, fixed in Go 1.13
							// https://git.io/fjXU6
							reflect.ValueOf(&aerr).Elem(),
						}
					}
				}

				return meth.Call([]reflect.Value{
					in[0], in[1], param,
				})
			}).Interface().(invokeFunc)

	}
	return code, nil
}

func DecodeParams(b []byte, out interface{}) error {
	um, ok := out.(cbg.CBORUnmarshaler)
	if !ok {
		return fmt.Errorf("type %T does not implement UnmarshalCBOR", out)
	}

	return um.UnmarshalCBOR(bytes.NewReader(b))
}

func DumpActorState(code cid.Cid, b []byte) (interface{}, error) {
	i := newInvoker() // TODO: register builtins in init block

	typ, ok := i.builtInState[code]
	if !ok {
		return nil, xerrors.Errorf("state type for actor %s not found", code)
	}

	rv := reflect.New(typ)
	um, ok := rv.Interface().(cbg.CBORUnmarshaler)
	if !ok {
		return nil, xerrors.New("state type does not implement CBORUnmarshaler")
	}

	if err := um.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return rv.Elem().Interface(), nil
}
