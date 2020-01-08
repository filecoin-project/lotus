package jsonrpc

import (
	"container/list"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
	"golang.org/x/xerrors"
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

type clientResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	ID      int64           `json:"id"`
	Error   *respError      `json:"error,omitempty"`
}

type makeChanSink func() (context.Context, func([]byte, bool))

type clientRequest struct {
	req   request
	ready chan clientResponse

	// retCh provides a context and sink for handling incoming channel messages
	retCh makeChanSink
}

// ClientCloser is used to close Client from further use
type ClientCloser func()

// NewClient creates new jsonrpc 2.0 client
//
// handler must be pointer to a struct with function fields
// Returned value closes the client connection
// TODO: Example
func NewClient(addr string, namespace string, handler interface{}, requestHeader http.Header) (ClientCloser, error) {
	return NewMergeClient(addr, namespace, []interface{}{handler}, requestHeader)
}

type client struct {
	namespace string

	requests chan clientRequest
	exiting  <-chan struct{}
	idCtr    int64
}

// NewMergeClient is like NewClient, but allows to specify multiple structs
// to be filled in the same namespace, using one connection
func NewMergeClient(addr string, namespace string, outs []interface{}, requestHeader http.Header) (ClientCloser, error) {
	conn, _, err := websocket.DefaultDialer.Dial(addr, requestHeader)
	if err != nil {
		return nil, err
	}

	c := client{
		namespace: namespace,
	}

	stop := make(chan struct{})
	exiting := make(chan struct{})
	c.requests = make(chan clientRequest)
	c.exiting = exiting

	handlers := map[string]rpcHandler{}
	go (&wsConn{
		conn:     conn,
		handler:  handlers,
		requests: c.requests,
		stop:     stop,
		exiting:  exiting,
	}).handleWsConn(context.TODO())

	for _, handler := range outs {
		htyp := reflect.TypeOf(handler)
		if htyp.Kind() != reflect.Ptr {
			return nil, xerrors.New("expected handler to be a pointer")
		}
		typ := htyp.Elem()
		if typ.Kind() != reflect.Struct {
			return nil, xerrors.New("handler should be a struct")
		}

		val := reflect.ValueOf(handler)

		for i := 0; i < typ.NumField(); i++ {
			fn, err := c.makeRpcFunc(typ.Field(i))
			if err != nil {
				return nil, err
			}

			val.Elem().Field(i).Set(fn)
		}
	}

	return func() {
		close(stop)
		<-exiting
	}, nil
}

func (c *client) makeOutChan(ctx context.Context, ftyp reflect.Type, valOut int) (func() reflect.Value, makeChanSink) {
	retVal := reflect.Zero(ftyp.Out(valOut))

	chCtor := func() (context.Context, func([]byte, bool)) {
		// unpack chan type to make sure it's reflect.BothDir
		ctyp := reflect.ChanOf(reflect.BothDir, ftyp.Out(valOut).Elem())
		ch := reflect.MakeChan(ctyp, 0) // todo: buffer?
		chCtx, chCancel := context.WithCancel(ctx)
		retVal = ch.Convert(ftyp.Out(valOut))

		buf := (&list.List{}).Init()
		var bufLk sync.Mutex

		return ctx, func(result []byte, ok bool) {
			if !ok {
				chCancel()
				// remote channel closed, close ours too
				ch.Close()
				return
			}

			val := reflect.New(ftyp.Out(valOut).Elem())
			if err := json.Unmarshal(result, val.Interface()); err != nil {
				log.Errorf("error unmarshaling chan response: %s", err)
				return
			}

			bufLk.Lock()
			if ctx.Err() != nil {
				log.Errorf("got rpc message with cancelled context: %s", ctx.Err())
				bufLk.Unlock()
				return
			}

			buf.PushBack(val)

			if buf.Len() > 1 {
				log.Warnw("rpc output message buffer", "n", buf.Len())
				bufLk.Unlock()
				return
			}

			go func() {
				for buf.Len() > 0 {
					front := buf.Front()
					bufLk.Unlock()

					cases := []reflect.SelectCase{
						{
							Dir:  reflect.SelectRecv,
							Chan: reflect.ValueOf(chCtx.Done()),
						},
						{
							Dir:  reflect.SelectSend,
							Chan: ch,
							Send: front.Value.(reflect.Value).Elem(),
						},
					}

					chosen, _, _ := reflect.Select(cases)
					bufLk.Lock()

					switch chosen {
					case 0:
						buf.Init()
					case 1:
						buf.Remove(front)
					}
				}

				bufLk.Unlock()
			}()

		}
	}

	return func() reflect.Value { return retVal }, chCtor
}

func (c *client) sendRequest(ctx context.Context, req request, chCtor makeChanSink) (clientResponse, error) {
	rchan := make(chan clientResponse, 1)
	creq := clientRequest{
		req:   req,
		ready: rchan,

		retCh: chCtor,
	}
	select {
	case c.requests <- creq:
	case <-c.exiting:
		return clientResponse{}, fmt.Errorf("websocket routine exiting")
	}

	var ctxDone <-chan struct{}
	var resp clientResponse

	if ctx != nil {
		ctxDone = ctx.Done()
	}

	// wait for response, handle context cancellation
loop:
	for {
		select {
		case resp = <-rchan:
			break loop
		case <-ctxDone: // send cancel request
			ctxDone = nil

			cancelReq := clientRequest{
				req: request{
					Jsonrpc: "2.0",
					Method:  wsCancel,
					Params:  []param{{v: reflect.ValueOf(*req.ID)}},
				},
			}
			select {
			case c.requests <- cancelReq:
			case <-c.exiting:
				log.Warn("failed to send request cancellation, websocket routing exited")
			}
		}
	}

	return resp, nil
}

type rpcFunc struct {
	client *client

	ftyp reflect.Type
	name string

	nout   int
	valOut int
	errOut int

	hasCtx int
	retCh  bool
}

func (fn *rpcFunc) processResponse(resp clientResponse, rval reflect.Value) []reflect.Value {
	out := make([]reflect.Value, fn.nout)

	if fn.valOut != -1 {
		out[fn.valOut] = rval
	}
	if fn.errOut != -1 {
		out[fn.errOut] = reflect.New(errorType).Elem()
		if resp.Error != nil {
			out[fn.errOut].Set(reflect.ValueOf(resp.Error))
		}
	}

	return out
}

func (fn *rpcFunc) processError(err error) []reflect.Value {
	out := make([]reflect.Value, fn.nout)

	if fn.valOut != -1 {
		out[fn.valOut] = reflect.New(fn.ftyp.Out(fn.valOut)).Elem()
	}
	if fn.errOut != -1 {
		out[fn.errOut] = reflect.New(errorType).Elem()
		out[fn.errOut].Set(reflect.ValueOf(&ErrClient{err}))
	}

	return out
}

func (fn *rpcFunc) handleRpcCall(args []reflect.Value) (results []reflect.Value) {
	id := atomic.AddInt64(&fn.client.idCtr, 1)
	params := make([]param, len(args)-fn.hasCtx)
	for i, arg := range args[fn.hasCtx:] {
		params[i] = param{
			v: arg,
		}
	}

	var ctx context.Context
	var span *trace.Span
	if fn.hasCtx == 1 {
		ctx = args[0].Interface().(context.Context)
		ctx, span = trace.StartSpan(ctx, "api.call")
		defer span.End()
	}

	retVal := func() reflect.Value { return reflect.Value{} }

	// if the function returns a channel, we need to provide a sink for the
	// messages
	var chCtor makeChanSink
	if fn.retCh {
		retVal, chCtor = fn.client.makeOutChan(ctx, fn.ftyp, fn.valOut)
	}

	req := request{
		Jsonrpc: "2.0",
		ID:      &id,
		Method:  fn.client.namespace + "." + fn.name,
		Params:  params,
	}

	if span != nil {
		span.AddAttributes(trace.StringAttribute("method", req.Method))

		eSC := base64.StdEncoding.EncodeToString(
			propagation.Binary(span.SpanContext()))
		req.Meta = map[string]string{
			"SpanContext": eSC,
		}
	}

	resp, err := fn.client.sendRequest(ctx, req, chCtor)
	if err != nil {
		return fn.processError(fmt.Errorf("sendRequest failed: %w", err))
	}

	if resp.ID != *req.ID {
		return fn.processError(xerrors.New("request and response id didn't match"))
	}

	if fn.valOut != -1 && !fn.retCh {
		val := reflect.New(fn.ftyp.Out(fn.valOut))

		if resp.Result != nil {
			log.Debugw("rpc result", "type", fn.ftyp.Out(fn.valOut))
			if err := json.Unmarshal(resp.Result, val.Interface()); err != nil {
				log.Warnw("unmarshaling failed", "message", string(resp.Result))
				return fn.processError(xerrors.Errorf("unmarshaling result: %w", err))
			}
		}

		retVal = func() reflect.Value { return val.Elem() }
	}

	return fn.processResponse(resp, retVal())
}

func (c *client) makeRpcFunc(f reflect.StructField) (reflect.Value, error) {
	ftyp := f.Type
	if ftyp.Kind() != reflect.Func {
		return reflect.Value{}, xerrors.New("handler field not a func")
	}

	fun := &rpcFunc{
		client: c,
		ftyp:   ftyp,
		name:   f.Name,
	}
	fun.valOut, fun.errOut, fun.nout = processFuncOut(ftyp)

	if ftyp.NumIn() > 0 && ftyp.In(0) == contextType {
		fun.hasCtx = 1
	}
	fun.retCh = fun.valOut != -1 && ftyp.Out(fun.valOut).Kind() == reflect.Chan

	return reflect.MakeFunc(ftyp, fun.handleRpcCall), nil
}
