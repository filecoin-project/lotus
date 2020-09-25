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
	vmr "github.com/filecoin-project/specs-actors/v2/actors/runtime"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type ActorRegistry struct {
	actors map[cid.Cid]*actorInfo
}

type invokeFunc func(rt vmr.Runtime, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc

type actorInfo struct {
	methods   nativeCode
	stateType reflect.Type
	// TODO: consider making this a network version range?
	version   actors.Version
	singleton bool
}

func NewActorRegistry() *ActorRegistry {
	inv := &ActorRegistry{actors: make(map[cid.Cid]*actorInfo)}

	// TODO: define all these properties on the actors themselves, in specs-actors.

	// add builtInCode using: register(cid, singleton)
	inv.Register(actors.Version0, builtin0.SystemActorCodeID, system0.Actor{}, abi.EmptyValue{}, true)
	inv.Register(actors.Version0, builtin0.InitActorCodeID, init0.Actor{}, init0.State{}, true)
	inv.Register(actors.Version0, builtin0.RewardActorCodeID, reward0.Actor{}, reward0.State{}, true)
	inv.Register(actors.Version0, builtin0.CronActorCodeID, cron0.Actor{}, cron0.State{}, true)
	inv.Register(actors.Version0, builtin0.StoragePowerActorCodeID, power0.Actor{}, power0.State{}, true)
	inv.Register(actors.Version0, builtin0.StorageMarketActorCodeID, market0.Actor{}, market0.State{}, true)
	inv.Register(actors.Version0, builtin0.VerifiedRegistryActorCodeID, verifreg0.Actor{}, verifreg0.State{}, true)
	inv.Register(actors.Version0, builtin0.StorageMinerActorCodeID, miner0.Actor{}, miner0.State{}, false)
	inv.Register(actors.Version0, builtin0.MultisigActorCodeID, msig0.Actor{}, msig0.State{}, false)
	inv.Register(actors.Version0, builtin0.PaymentChannelActorCodeID, paych0.Actor{}, paych0.State{}, false)
	inv.Register(actors.Version0, builtin0.AccountActorCodeID, account0.Actor{}, account0.State{}, false)

	inv.Register(actors.Version1, builtin1.SystemActorCodeID, system1.Actor{}, abi.EmptyValue{}, true)
	inv.Register(actors.Version1, builtin1.InitActorCodeID, init1.Actor{}, init1.State{}, true)
	inv.Register(actors.Version1, builtin1.RewardActorCodeID, reward1.Actor{}, reward1.State{}, true)
	inv.Register(actors.Version1, builtin1.CronActorCodeID, cron1.Actor{}, cron1.State{}, true)
	inv.Register(actors.Version1, builtin1.StoragePowerActorCodeID, power1.Actor{}, power1.State{}, true)
	inv.Register(actors.Version1, builtin1.StorageMarketActorCodeID, market1.Actor{}, market1.State{}, true)
	inv.Register(actors.Version1, builtin1.VerifiedRegistryActorCodeID, verifreg1.Actor{}, verifreg1.State{}, true)
	inv.Register(actors.Version1, builtin1.StorageMinerActorCodeID, miner1.Actor{}, miner1.State{}, false)
	inv.Register(actors.Version1, builtin1.MultisigActorCodeID, msig1.Actor{}, msig1.State{}, false)
	inv.Register(actors.Version1, builtin1.PaymentChannelActorCodeID, paych1.Actor{}, paych1.State{}, false)
	inv.Register(actors.Version1, builtin1.AccountActorCodeID, account1.Actor{}, account1.State{}, false)

	return inv
}

func (ar *ActorRegistry) Invoke(codeCid cid.Cid, rt vmr.Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {
	act, ok := ar.actors[codeCid]
	if !ok {
		log.Errorf("no code for actor %s (Addr: %s)", codeCid, rt.Receiver())
		return nil, aerrors.Newf(exitcode.SysErrorIllegalActor, "no code for actor %s(%d)(%s)", codeCid, method, hex.EncodeToString(params))
	}
	if method >= abi.MethodNum(len(act.methods)) || act.methods[method] == nil {
		return nil, aerrors.Newf(exitcode.SysErrInvalidMethod, "no method %d on actor", method)
	}
	if curVer := actors.VersionForNetwork(rt.NetworkVersion()); curVer != act.version {
		return nil, aerrors.Newf(exitcode.SysErrInvalidMethod, "unsupported actors code version %d, expected %d", act.version, curVer)
	}
	return act.methods[method](rt, params)

}

func (ar *ActorRegistry) Register(version actors.Version, c cid.Cid, instance Invokee, state interface{}, singleton bool) {
	code, err := ar.transform(instance)
	if err != nil {
		panic(xerrors.Errorf("%s: %w", string(c.Hash()), err))
	}
	ar.actors[c] = &actorInfo{
		methods:   code,
		version:   version,
		stateType: reflect.TypeOf(state),
		singleton: singleton,
	}
}

func (ar *ActorRegistry) Create(codeCid cid.Cid, rt vmr.Runtime) (*types.Actor, aerrors.ActorError) {
	act, ok := ar.actors[codeCid]
	if !ok {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalArgument, "Can only create built-in actors.")
	}
	if version := actors.VersionForNetwork(rt.NetworkVersion()); act.version != version {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalArgument, "Can only create version %d actors, attempted to create version %d actor", version, act.version)
	}

	if act.singleton {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalArgument, "Can only have one instance of singleton actors.")
	}
	return &types.Actor{
		Code:    codeCid,
		Head:    EmptyObjectCid,
		Nonce:   0,
		Balance: abi.NewTokenAmount(0),
	}, nil
}

type Invokee interface {
	Exports() []interface{}
}

func (*ActorRegistry) transform(instance Invokee) (nativeCode, error) {
	itype := reflect.TypeOf(instance)
	exports := instance.Exports()
	runtimeType := reflect.TypeOf((*vmr.Runtime)(nil)).Elem()
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
		if !runtimeType.Implements(t.In(0)) {
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

func DumpActorState(act *types.Actor, b []byte) (interface{}, error) {
	if act.IsAccountActor() { // Account code special case
		return nil, nil
	}

	i := NewActorRegistry() // TODO: register builtins in init block

	actInfo, ok := i.actors[act.Code]
	if !ok {
		return nil, xerrors.Errorf("state type for actor %s not found", act.Code)
	}

	rv := reflect.New(actInfo.stateType)
	um, ok := rv.Interface().(cbg.CBORUnmarshaler)
	if !ok {
		return nil, xerrors.New("state type does not implement CBORUnmarshaler")
	}

	if err := um.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("unmarshaling actor state: %w", err)
	}

	return rv.Elem().Interface(), nil
}
