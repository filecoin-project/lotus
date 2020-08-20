package vm

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/cron"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/system"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
)

type Invoker struct {
	builtInCode  map[cid.Cid]nativeCode
	builtInState map[cid.Cid]reflect.Type
}

type invokeFunc func(rt runtime.Runtime, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc

func NewInvoker() *Invoker {
	inv := &Invoker{
		builtInCode:  make(map[cid.Cid]nativeCode),
		builtInState: make(map[cid.Cid]reflect.Type),
	}

	// add builtInCode using: register(cid, singleton)
	inv.Register(builtin.SystemActorCodeID, system.Actor{}, adt.EmptyValue{})
	inv.Register(builtin.InitActorCodeID, init_.Actor{}, init_.State{})
	inv.Register(builtin.RewardActorCodeID, reward.Actor{}, reward.State{})
	inv.Register(builtin.CronActorCodeID, cron.Actor{}, cron.State{})
	inv.Register(builtin.StoragePowerActorCodeID, power.Actor{}, power.State{})
	inv.Register(builtin.StorageMarketActorCodeID, market.Actor{}, market.State{})
	inv.Register(builtin.StorageMinerActorCodeID, miner.Actor{}, miner.State{})
	inv.Register(builtin.MultisigActorCodeID, multisig.Actor{}, multisig.State{})
	inv.Register(builtin.PaymentChannelActorCodeID, paych.Actor{}, paych.State{})
	inv.Register(builtin.VerifiedRegistryActorCodeID, verifreg.Actor{}, verifreg.State{})
	inv.Register(builtin.AccountActorCodeID, account.Actor{}, account.State{})

	return inv
}

func (inv *Invoker) Invoke(codeCid cid.Cid, rt runtime.Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {

	code, ok := inv.builtInCode[codeCid]
	if !ok {
		log.Errorf("no code for actor %s (Addr: %s)", codeCid, rt.Message().Receiver())
		return nil, aerrors.Newf(exitcode.SysErrorIllegalActor, "no code for actor %s(%d)(%s)", codeCid, method, hex.EncodeToString(params))
	}
	if method >= abi.MethodNum(len(code)) || code[method] == nil {
		return nil, aerrors.Newf(exitcode.SysErrInvalidMethod, "no method %d on actor", method)
	}
	return code[method](rt, params)

}

func (inv *Invoker) Register(c cid.Cid, instance Invokee, state interface{}) {
	code, err := inv.transform(instance)
	if err != nil {
		panic(xerrors.Errorf("%s: %w", string(c.Hash()), err))
	}
	inv.builtInCode[c] = code
	inv.builtInState[c] = reflect.TypeOf(state)
}

type Invokee interface {
	Exports() []interface{}
}

func (*Invoker) transform(instance Invokee) (nativeCode, error) {
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
		if t.NumIn() != 2 {
			return nil, newErr("wrong number of inputs should be: " +
				"vmr.Runtime, <parameter>")
		}
		if t.In(0) != reflect.TypeOf((*vmr.Runtime)(nil)).Elem() {
			return nil, newErr("first arguemnt should be vmr.Runtime")
		}
		if t.In(1).Kind() != reflect.Ptr {
			return nil, newErr("second argument should be Runtime")
		}

		if t.NumOut() != 1 {
			return nil, newErr("wrong number of outputs should be: " +
				"cbg.CBORMarshaler")
		}
		o0 := t.Out(0)
		if !o0.Implements(reflect.TypeOf((*cbg.CBORMarshaler)(nil)).Elem()) {
			return nil, newErr("output needs to implement cgb.CBORMarshaler")
		}
	}
	code := make(nativeCode, len(exports))
	for id, m := range exports {
		meth := reflect.ValueOf(m)
		code[id] = reflect.MakeFunc(reflect.TypeOf((invokeFunc)(nil)),
			func(in []reflect.Value) []reflect.Value {
				paramT := meth.Type().In(1).Elem()
				param := reflect.New(paramT)

				inBytes := in[1].Interface().([]byte)
				if err := DecodeParams(inBytes, param.Interface()); err != nil {
					aerr := aerrors.Absorb(err, 1, "failed to decode parameters")
					return []reflect.Value{
						reflect.ValueOf([]byte{}),
						// Below is a hack, fixed in Go 1.13
						// https://git.io/fjXU6
						reflect.ValueOf(&aerr).Elem(),
					}
				}
				rt := in[0].Interface().(*Runtime)
				rval, aerror := rt.shimCall(func() interface{} {
					ret := meth.Call([]reflect.Value{
						reflect.ValueOf(rt),
						param,
					})
					return ret[0].Interface()
				})

				return []reflect.Value{
					reflect.ValueOf(&rval).Elem(),
					reflect.ValueOf(&aerror).Elem(),
				}
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
	if code == builtin.AccountActorCodeID { // Account code special case
		return nil, nil
	}

	i := NewInvoker() // TODO: register builtins in init block

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
		return nil, xerrors.Errorf("unmarshaling actor state: %w", err)
	}

	return rv.Elem().Interface(), nil
}
