package rpclib

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
)

type result reflect.Value

func (r *result) UnmarshalJSON(raw []byte) error {
	return json.Unmarshal(raw, reflect.Value(*r).Interface())
}

type clientResponse struct {
	Jsonrpc string     `json:"jsonrpc"`
	Result  result     `json:"result"`
	Id      int64      `json:"id"`
	Error   *respError `json:"error,omitempty"`
}

func NewClient(addr string, namespace string, handler interface{}) {
	htyp := reflect.TypeOf(handler)
	if htyp.Kind() != reflect.Ptr {
		panic("expected handler to be a pointer")
	}
	typ := htyp.Elem()
	if typ.Kind() != reflect.Struct {
		panic("handler should be a struct")
	}

	val := reflect.ValueOf(handler)

	var idCtr int64

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		ftyp := f.Type
		if ftyp.Kind() != reflect.Func {
			panic("handler field not a func")
		}

		valOut, errOut, nout := processFuncOut(ftyp)

		processResponse := func(resp clientResponse) []reflect.Value {
			out := make([]reflect.Value, nout)

			if valOut != -1 {
				out[valOut] = reflect.Value(resp.Result).Elem()
			}
			if errOut != -1 {
				out[errOut] = reflect.New(reflect.TypeOf(new(error)).Elem()).Elem()
				if resp.Error != nil {
					out[errOut].Set(reflect.ValueOf(errors.New(resp.Error.Message)))
				}
			}

			return out
		}

		fn := reflect.MakeFunc(ftyp, func(args []reflect.Value) (results []reflect.Value) {
			id := atomic.AddInt64(&idCtr, 1)
			params := make([]param, len(args))
			for i, arg := range args {
				params[i] = param{
					v: arg,
				}
			}

			req := request{
				Jsonrpc: "2.0",
				Id:      &id,
				Method:  namespace + "." + f.Name,
				Params:  params,
			}

			b, err := json.Marshal(&req)
			if err != nil {
				// TODO: try returning an error if the method has one
				panic(err)
			}

			httpResp, err := http.Post(addr, "application/json", bytes.NewReader(b))
			if err != nil {
				// TODO: try returning an error if the method has one
				panic(err)
			}
			defer httpResp.Body.Close()

			if httpResp.StatusCode != 200 {
				// TODO: try returning an error if the method has one
				// TODO: actually parse response, it haz right errors
				panic("non 200 code")
			}

			var resp clientResponse
			if valOut != -1 {
				resp.Result = result(reflect.New(ftyp.Out(valOut)))
			}

			if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
				// TODO: try returning an error if the method has one
				panic(err)
			}

			if resp.Id != *req.Id {
				// TODO: try returning an error if the method has one
				panic("request and response id didn't match")
			}

			return processResponse(resp)
		})

		val.Elem().Field(i).Set(fn)
	}
}
