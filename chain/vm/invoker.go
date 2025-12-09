package vm

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtinst "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	vmr "github.com/filecoin-project/specs-actors/v7/actors/runtime"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

type MethodMeta struct {
	Name string

	Params reflect.Type
	Ret    reflect.Type
}

type ActorRegistry struct {
	actors map[cid.Cid]*actorInfo

	Methods map[cid.Cid]map[abi.MethodNum]MethodMeta
}

// An ActorPredicate returns an error if the given actor is not valid for the given runtime environment (e.g., chain height, version, etc.).
type ActorPredicate func(vmr.Runtime, cid.Cid) error

func ActorsVersionPredicate(ver actorstypes.Version) ActorPredicate {
	return func(rt vmr.Runtime, codeCid cid.Cid) error {
		aver, err := actorstypes.VersionForNetwork(rt.NetworkVersion())
		if err != nil {
			return xerrors.Errorf("unsupported network version: %w", err)
		}
		if aver != ver {
			return xerrors.Errorf("actor %s is a version %d actor; chain only supports actor version %d at height %d and nver %d", codeCid, ver, aver, rt.CurrEpoch(), rt.NetworkVersion())
		}
		return nil
	}
}

type invokeFunc func(rt vmr.Runtime, params []byte) ([]byte, aerrors.ActorError)
type nativeCode map[abi.MethodNum]invokeFunc

type actorInfo struct {
	methods nativeCode
	vmActor builtin.RegistryEntry
	// TODO: consider making this a network version range?
	predicate ActorPredicate
}

func NewActorRegistry() *ActorRegistry {
	return &ActorRegistry{
		actors:  make(map[cid.Cid]*actorInfo),
		Methods: map[cid.Cid]map[abi.MethodNum]MethodMeta{},
	}
}

func (ar *ActorRegistry) Invoke(codeCid cid.Cid, rt vmr.Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {
	act, ok := ar.actors[codeCid]
	if !ok {
		log.Errorf("no code for actor %s (Addr: %s)", codeCid, rt.Receiver())
		return nil, aerrors.Newf(exitcode.SysErrorIllegalActor, "no code for actor %s(%d)(%s)", codeCid, method, hex.EncodeToString(params))
	}
	if err := act.predicate(rt, codeCid); err != nil {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalActor, "unsupported actor: %s", err)
	}
	if act.methods[method] == nil {
		return nil, aerrors.Newf(exitcode.SysErrInvalidMethod, "no method %d on actor", method)
	}
	return act.methods[method](rt, params)

}

func (ar *ActorRegistry) Register(av actorstypes.Version, pred ActorPredicate, vmactors []builtin.RegistryEntry) {
	if pred == nil {
		pred = func(vmr.Runtime, cid.Cid) error { return nil }
	}
	for _, a := range vmactors {

		var code nativeCode
		var err error
		if av <= actorstypes.Version7 {
			// register in the `actors` map (for the invoker)
			code, err = ar.transform(a)
			if err != nil {
				panic(xerrors.Errorf("%s: %w", string(a.Code().Hash()), err))
			}
		}

		ai := &actorInfo{
			methods:   code,
			vmActor:   a,
			predicate: pred,
		}

		ac := a.Code()
		ar.actors[ac] = ai

		// necessary to make stuff work
		var realCode cid.Cid
		if av >= actorstypes.Version8 {
			name := actors.CanonicalName(builtin.ActorNameByCode(ac))

			var ok bool
			realCode, ok = actors.GetActorCodeID(av, name)
			if ok {
				ar.actors[realCode] = ai
			}
		}

		// register in the `Methods` map (used by statemanager utils)
		exports := a.Exports()
		methods := make(map[abi.MethodNum]MethodMeta, len(exports))

		// Explicitly add send, it's special.
		methods[builtin.MethodSend] = MethodMeta{
			Name:   "Send",
			Params: reflect.TypeFor[*abi.EmptyValue](),
			Ret:    reflect.TypeFor[*abi.EmptyValue](),
		}

		// Iterate over exported methods. Some of these _may_ be nil and
		// must be skipped.
		for number, export := range exports {
			if export.Method == nil {
				continue
			}

			ev := reflect.ValueOf(export.Method)
			et := ev.Type()

			mm := MethodMeta{
				Name: export.Name,
				Ret:  et.Out(0),
			}

			if av <= actorstypes.Version7 {
				// methods exported from specs-actors have the runtime as the first param, so we want et.In(1)
				mm.Params = et.In(1)
			} else {
				// methods exported from go-state-types do not, so we want et.In(0)
				mm.Params = et.In(0)
			}

			methods[number] = mm
		}
		if realCode.Defined() {
			ar.Methods[realCode] = methods
		} else {
			ar.Methods[a.Code()] = methods
		}
	}
}

func (ar *ActorRegistry) Create(codeCid cid.Cid, rt vmr.Runtime) (*types.Actor, aerrors.ActorError) {
	act, ok := ar.actors[codeCid]
	if !ok {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalArgument, "Can only create built-in actors.")
	}

	if err := act.predicate(rt, codeCid); err != nil {
		return nil, aerrors.Newf(exitcode.SysErrorIllegalArgument, "Cannot create actor: %w", err)
	}

	return &types.Actor{
		Code:    codeCid,
		Head:    EmptyObjectCid,
		Nonce:   0,
		Balance: abi.NewTokenAmount(0),
	}, nil
}

type invokee interface {
	Exports() map[abi.MethodNum]builtinst.MethodMeta
}

func (*ActorRegistry) transform(instance invokee) (nativeCode, error) {
	itype := reflect.TypeOf(instance)
	exports := instance.Exports()
	runtimeType := reflect.TypeFor[vmr.Runtime]()
	for i, e := range exports {
		i := i
		m := e.Method
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
			return nil, newErr("first argument should be vmr.Runtime")
		}
		if t.In(1).Kind() != reflect.Ptr {
			return nil, newErr("second argument should be of kind reflect.Ptr")
		}

		if t.NumOut() != 1 {
			return nil, newErr("wrong number of outputs should be: " +
				"cbg.CBORMarshaler")
		}
		o0 := t.Out(0)
		if !o0.Implements(reflect.TypeFor[cbg.CBORMarshaler]()) {
			return nil, newErr("output needs to implement cgb.CBORMarshaler")
		}
	}
	code := make(nativeCode, len(exports))
	for id, e := range exports {
		m := e.Method
		if m == nil {
			continue
		}
		meth := reflect.ValueOf(m)
		code[id] = reflect.MakeFunc(reflect.TypeFor[invokeFunc](),
			func(in []reflect.Value) []reflect.Value {
				paramT := meth.Type().In(1).Elem()
				param := reflect.New(paramT)

				rt := in[0].Interface().(*Runtime)
				inBytes := in[1].Interface().([]byte)
				if err := DecodeParams(inBytes, param.Interface()); err != nil {
					ec := exitcode.ErrSerialization
					if rt.NetworkVersion() < network.Version7 {
						ec = 1
					}
					aerr := aerrors.Absorb(err, ec, "failed to decode parameters")
					return []reflect.Value{
						reflect.ValueOf([]byte{}),
						// Below is a hack, fixed in Go 1.13
						// https://git.io/fjXU6
						reflect.ValueOf(&aerr).Elem(),
					}
				}
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

func DumpActorState(i *ActorRegistry, act *types.Actor, b []byte) (interface{}, error) {
	actInfo, ok := i.actors[act.Code]
	if !ok {
		return nil, xerrors.Errorf("state type for actor %s not found", act.Code)
	}

	um := actInfo.vmActor.State()
	if um == nil {
		if act.Head != EmptyObjectCid {
			return nil, xerrors.Errorf("actor with code %s should only have empty object (%s) as its Head, instead has %s", act.Code, EmptyObjectCid, act.Head)
		}

		return nil, nil
	}
	if err := um.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("unmarshaling actor state: %w", err)
	}

	return um, nil
}
