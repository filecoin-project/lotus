package vm

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	account0 "github.com/filecoin-project/specs-actors/actors/builtin/account"
	cron0 "github.com/filecoin-project/specs-actors/actors/builtin/cron"
	init0 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	paych0 "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	system0 "github.com/filecoin-project/specs-actors/actors/builtin/system"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	builtin1 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	account1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/account"
	cron1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/cron"
	init1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	market1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	miner1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	msig1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"
	paych1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/paych"
	power1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	reward1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/reward"
	system1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/system"
	verifreg1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
)

type Invoker struct {
	builtInCode  map[cid.Cid]nativeCode
	builtInState map[cid.Cid]reflect.Type
}

type invokeFunc func(rt vmr.Runtime, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc

func NewInvoker() *Invoker {
	inv := &Invoker{
		builtInCode:  make(map[cid.Cid]nativeCode),
		builtInState: make(map[cid.Cid]reflect.Type),
	}

	// add builtInCode using: register(cid, singleton)
	inv.Register(builtin0.SystemActorCodeID, system0.Actor{}, abi.EmptyValue{})
	inv.Register(builtin0.InitActorCodeID, init0.Actor{}, init0.State{})
	inv.Register(builtin0.RewardActorCodeID, reward0.Actor{}, reward0.State{})
	inv.Register(builtin0.CronActorCodeID, cron0.Actor{}, cron0.State{})
	inv.Register(builtin0.StoragePowerActorCodeID, power0.Actor{}, power0.State{})
	inv.Register(builtin0.StorageMarketActorCodeID, market0.Actor{}, market0.State{})
	inv.Register(builtin0.StorageMinerActorCodeID, miner0.Actor{}, miner0.State{})
	inv.Register(builtin0.MultisigActorCodeID, msig0.Actor{}, msig0.State{})
	inv.Register(builtin0.PaymentChannelActorCodeID, paych0.Actor{}, paych0.State{})
	inv.Register(builtin0.VerifiedRegistryActorCodeID, verifreg0.Actor{}, verifreg0.State{})
	inv.Register(builtin0.AccountActorCodeID, account0.Actor{}, account0.State{})

	inv.Register(builtin1.SystemActorCodeID, system1.Actor{}, abi.EmptyValue{})
	inv.Register(builtin1.InitActorCodeID, init1.Actor{}, init1.State{})
	inv.Register(builtin1.RewardActorCodeID, reward1.Actor{}, reward1.State{})
	inv.Register(builtin1.CronActorCodeID, cron1.Actor{}, cron1.State{})
	inv.Register(builtin1.StoragePowerActorCodeID, power1.Actor{}, power1.State{})
	inv.Register(builtin1.StorageMarketActorCodeID, market1.Actor{}, market1.State{})
	inv.Register(builtin1.StorageMinerActorCodeID, miner1.Actor{}, miner1.State{})
	inv.Register(builtin1.MultisigActorCodeID, msig1.Actor{}, msig1.State{})
	inv.Register(builtin1.PaymentChannelActorCodeID, paych1.Actor{}, paych1.State{})
	inv.Register(builtin1.VerifiedRegistryActorCodeID, verifreg1.Actor{}, verifreg1.State{})
	inv.Register(builtin1.AccountActorCodeID, account1.Actor{}, account1.State{})

	return inv
}

func (inv *Invoker) Invoke(codeCid cid.Cid, rt vmr.Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {

	code, ok := inv.builtInCode[codeCid]
	if !ok {
		log.Errorf("no code for actor %s (Addr: %s)", codeCid, rt.Receiver())
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
			return nil, newErr("second argument should be of kind reflect.Ptr")
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
		if m == nil {
			continue
		}
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
	if code == builtin0.AccountActorCodeID { // Account code special case
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
