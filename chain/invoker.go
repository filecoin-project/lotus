package chain

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
)

type invoker struct {
	builtInCode map[cid.Cid]nativeCode
}

type invokeFunc func(act *Actor, vmctx *VMContext, params []byte) (InvokeRet, error)
type nativeCode []invokeFunc
type InvokeRet struct {
	result     []byte
	returnCode byte
}

func newInvoker() *invoker {
	inv := &invoker{
		builtInCode: make(map[cid.Cid]nativeCode),
	}
	// add builtInCode using: register(cid, singleton)
	return inv
}

func (inv *invoker) Invoke(act *Actor, vmctx *VMContext, method uint64, params []byte) (InvokeRet, error) {

	code, ok := inv.builtInCode[act.Code]
	if !ok {
		return InvokeRet{}, errors.New("no code for actor")
	}
	if method >= uint64(len(code)) || code[method] == nil {
		return InvokeRet{}, fmt.Errorf("no method %d on actor", method)
	}
	return code[method](act, vmctx, params)

}

func (inv *invoker) register(c cid.Cid, instance interface{}) {
	code, err := inv.transform(instance)
	if err != nil {
		panic(err)
	}
	inv.builtInCode[c] = code
}

type unmarshalCBOR interface {
	UnmarshalCBOR([]byte) (int, error)
}

var tUnmarhsalCBOR = reflect.TypeOf((*unmarshalCBOR)(nil)).Elem()
var tError = reflect.TypeOf((*error)(nil)).Elem()

func (_ *invoker) transform(instance interface{}) (nativeCode, error) {
	itype := reflect.TypeOf(instance)
	newErr := func(str string) error {
		return fmt.Errorf("transform(%s): %s", itype.Name(), str)

	}
	var maxn uint64
	invokes := make(map[uint64]reflect.Method)
	for i := 0; i < itype.NumMethod(); i++ {
		meth := itype.Method(i)
		if !strings.HasPrefix(meth.Name, "Invoke") {
			continue
		}
		sid := strings.TrimLeftFunc(meth.Name, func(r rune) bool {
			return r < '0' || r > '9'
		})

		id, err := strconv.ParseUint(sid, 10, 64)
		if err != nil {
			return nil, err
		}

		t := meth.Type
		if t.NumIn() != 4 {
			return nil, newErr("wrong number of inputs should be: " +
				"*Actor, *VMContext, <type of parameter>")
		}
		if t.In(0) != itype {
			return nil, newErr("passed instance is not struct")
		}
		if t.In(1) != reflect.TypeOf(&Actor{}) {
			return nil, newErr("first arguemnt should be *Actor")
		}
		if t.In(2) != reflect.TypeOf(&VMContext{}) {
			return nil, newErr("second argument should be *VMContext")
		}

		if !t.In(3).Implements(tUnmarhsalCBOR) {
			return nil, newErr("paramter doesn't implement UnmarshalCBOR")
		}

		if t.In(3).Kind() != reflect.Ptr {
			return nil, newErr("paramter has to be a pointer")
		}

		if t.NumOut() != 2 {
			return nil, newErr("wrong number of outputs should be: " +
				"(InvokeRet, error)")
		}
		if t.Out(0) != reflect.TypeOf(InvokeRet{}) {
			return nil, newErr("first output should be of type InvokeRet")
		}
		if !t.Out(1).Implements(tError) {
			return nil, newErr("second output should be error type")
		}

		if id > maxn {
			maxn = id
		}
		if _, has := invokes[id]; has {
			return nil, newErr(fmt.Sprintf("repeated method=%s id: %d", meth.Name, id))
		}
		invokes[id] = meth
	}
	code := make(nativeCode, maxn+1)
	_ = code
	for id, meth := range invokes {
		code[id] = reflect.MakeFunc(reflect.TypeOf((invokeFunc)(nil)),
			func(in []reflect.Value) []reflect.Value {
				paramT := meth.Type.In(3).Elem()
				param := reflect.New(paramT)

				param.Interface().(unmarshalCBOR).UnmarshalCBOR(in[2].Interface().([]byte))
				return meth.Func.Call([]reflect.Value{
					reflect.ValueOf(instance), in[0], in[1], param,
				})
			}).Interface().(invokeFunc)

	}
	return code, nil
}
