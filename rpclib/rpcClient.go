package rpclib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
)

var (
	errorType   = reflect.TypeOf(new(error)).Elem()
	contextType = reflect.TypeOf(new(context.Context)).Elem()
)

type ErrClient struct {
	err error
}

func (e *ErrClient) Error() string {
	return fmt.Sprintf("RPC client error: %s", e.err)
}

func (e *ErrClient) Unwrap(err error) error {
	return e.err
}

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
				out[errOut] = reflect.New(errorType).Elem()
				if resp.Error != nil {
					out[errOut].Set(reflect.ValueOf(errors.New(resp.Error.Message)))
				}
			}

			return out
		}

		processError := func(err error) []reflect.Value {
			out := make([]reflect.Value, nout)

			if valOut != -1 {
				out[valOut] = reflect.New(ftyp.Out(valOut)).Elem()
			}
			if errOut != -1 {
				out[errOut] = reflect.New(errorType).Elem()
				out[errOut].Set(reflect.ValueOf(&ErrClient{err}))
			}

			return out
		}

		hasCtx := 0
		if ftyp.NumIn() > 0 && ftyp.In(0) == contextType {
			hasCtx = 1
		}

		fn := reflect.MakeFunc(ftyp, func(args []reflect.Value) (results []reflect.Value) {
			id := atomic.AddInt64(&idCtr, 1)
			params := make([]param, len(args)-hasCtx)
			for i, arg := range args[hasCtx:] {
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
				return processError(err)
			}

			// prepare / execute http request

			hreq, err := http.NewRequest("POST", addr, bytes.NewReader(b))
			if err != nil {
				return processError(err)
			}
			if hasCtx == 1 {
				hreq = hreq.WithContext(args[0].Interface().(context.Context))
			}
			hreq.Header.Set("Content-Type", "application/json")

			httpResp, err := http.DefaultClient.Do(hreq)
			if err != nil {
				return processError(err)
			}
			defer httpResp.Body.Close()

			// process response

			// TODO: check error codes in spec
			if httpResp.StatusCode != 200 {
				return processError(errors.New("non 200 response code"))
			}

			var resp clientResponse
			if valOut != -1 {
				resp.Result = result(reflect.New(ftyp.Out(valOut)))
			}

			if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
				return processError(err)
			}

			if resp.Id != *req.Id {
				return processError(errors.New("request and response id didn't match"))
			}

			return processResponse(resp)
		})

		val.Elem().Field(i).Set(fn)
	}
}
