package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/gorilla/websocket"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("rpc")

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

type result []byte

func (p *result) UnmarshalJSON(raw []byte) error {
	*p = make([]byte, len(raw))
	copy(*p, raw)
	return nil
}

type clientResponse struct {
	Jsonrpc string     `json:"jsonrpc"`
	Result  result     `json:"result"`
	ID      int64      `json:"id"`
	Error   *respError `json:"error,omitempty"`
}

type clientRequest struct {
	req   request
	ready chan clientResponse
}

// ClientCloser is used to close Client from further use
type ClientCloser func()

// NewClient creates new josnrpc 2.0 client
//
// handler must be pointer to a struct with function fields
// Returned value closes the client connection
// TODO: Example
func NewClient(addr string, namespace string, handler interface{}) (ClientCloser, error) {
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

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	requests := make(chan clientRequest)

	handlers := map[string]rpcHandler{}
	go handleWsConn(context.TODO(), conn, handlers, requests, stop)

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		ftyp := f.Type
		if ftyp.Kind() != reflect.Func {
			panic("handler field not a func")
		}

		valOut, errOut, nout := processFuncOut(ftyp)

		processResponse := func(resp clientResponse, rval reflect.Value) []reflect.Value {
			out := make([]reflect.Value, nout)

			if valOut != -1 {
				out[valOut] = rval.Elem()
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
			params := make([]param, len(args)-hasCtx)
			for i, arg := range args[hasCtx:] {
				params[i] = param{
					v: arg,
				}
			}

			req := request{
				Jsonrpc: "2.0",
				ID:      &id,
				Method:  namespace + "." + f.Name,
				Params:  params,
			}

			rchan := make(chan clientResponse, 1)
			requests <- clientRequest{
				req:   req,
				ready: rchan,
			}
			resp := <- rchan
			var rval reflect.Value

			if valOut != -1 {
				log.Debugw("rpc result", "type", ftyp.Out(valOut))
				rval = reflect.New(ftyp.Out(valOut))
				if err := json.Unmarshal(resp.Result, rval.Interface()); err != nil {
					return processError(err)
				}
			}

			if resp.ID != *req.ID {
				return processError(errors.New("request and response id didn't match"))
			}

			return processResponse(resp, rval)
		})

		val.Elem().Field(i).Set(fn)
	}

	return func() {
		close(stop)
	}, nil
}
