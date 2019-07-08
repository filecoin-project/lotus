package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/filecoin-project/go-lotus/lib"
	"net/http"
	"reflect"
	"sync/atomic"
)

var (
	errorType   = reflect.TypeOf(new(error)).Elem()
	contextType = reflect.TypeOf(new(context.Context)).Elem()
)

// ErrClient is an error which occurred on the client side the library
type ErrClient struct {
	err error
}

func (e *ErrClient) Error() string {
	return fmt.Sprintf("RPC client error: %s", e.err)
}

// Unwrap unwraps the actual error
func (e *ErrClient) Unwrap(err error) error {
	return e.err
}

type result reflect.Value

func (r *result) UnmarshalJSON(raw []byte) error {
	return json.Unmarshal(raw, reflect.Value(*r).Interface())
}

type clientResponse struct {
	Jsonrpc string         `json:"jsonrpc"`
	Result  result         `json:"result"`
	ID      int64          `json:"id"`
	Error   *lib.respError `json:"error,omitempty"`
}

// ClientCloser is used to close Client from further use
type ClientCloser func()

// NewClient creates new josnrpc 2.0 client
//
// handler must be pointer to a struct with function fields
// Returned value closes the client connection
// TODO: Example
func NewClient(addr string, namespace string, handler interface{}) ClientCloser {
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

		valOut, errOut, nout := lib.processFuncOut(ftyp)

		processResponse := func(resp clientResponse, code int) []reflect.Value {
			out := make([]reflect.Value, nout)

			if valOut != -1 {
				out[valOut] = reflect.Value(resp.Result).Elem()
			}
			if errOut != -1 {
				out[errOut] = reflect.New(errorType).Elem()
				if resp.Error != nil {
					out[errOut].Set(reflect.ValueOf(resp.Error))
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
			params := make([]lib.param, len(args)-hasCtx)
			for i, arg := range args[hasCtx:] {
				params[i] = lib.param{
					v: arg,
				}
			}

			req := lib.request{
				Jsonrpc: "2.0",
				ID:      &id,
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

			// process response

			var resp clientResponse
			if valOut != -1 {
				resp.Result = result(reflect.New(ftyp.Out(valOut)))
			}

			if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
				return processError(err)
			}

			if err := httpResp.Body.Close(); err != nil {
				return processError(err)
			}

			if resp.ID != *req.ID {
				return processError(errors.New("request and response id didn't match"))
			}

			return processResponse(resp, httpResp.StatusCode)
		})

		val.Elem().Field(i).Set(fn)
	}

	// TODO: if this is still unused as of 2020, remove the closer stuff
	return func() {} // noop for now, not for long though
}
