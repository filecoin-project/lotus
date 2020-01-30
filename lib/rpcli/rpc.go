package rpcli

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"sync"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
	"golang.org/x/xerrors"
)

var nullValue = reflect.ValueOf(struct{}{})

type rpcContext struct {
	id     int64
	ctx    context.Context
	cancel context.CancelFunc

	req    frame
	isChan bool

	opt CallOption

	// we take the advantage of mutex fairness since go1.12 here
	// to keep incoming dataOrErrs ordered
	// and since there can be only 1 receiver, no need to acquire mutex before consuming
	resChMu   sync.Mutex
	resCh     chan dataOrErr
	logger    logging.StandardLogger
	streaming bool
}

func (rc *rpcContext) send(de dataOrErr) {
	if contextDone(rc.ctx) {
		return
	}

	started := make(chan struct{}, 0)

	go func() {
		close(started)

		rc.resChMu.Lock()
		defer rc.resChMu.Unlock()

		select {
		case <-rc.ctx.Done():
			return

		case rc.resCh <- de:
			return
		}
	}()

	<-started
}

func (rc *rpcContext) finish(connID *uint64, err error) {
	if contextDone(rc.ctx) {
		return
	}

	started := make(chan struct{}, 0)

	go func() {
		close(started)

		rc.resChMu.Lock()
		defer func() {
			rc.cancel()
			rc.resChMu.Unlock()
		}()

		if err != nil {
			select {
			case <-rc.ctx.Done():
				return

			case rc.resCh <- dataOrErr{
				connID: connID,
				err:    err,
			}:
				return
			}
		}

	}()

	<-started
}

func newRPCMethod(field reflect.StructField, cli Client, idgen idGenerator, namespace string) (reflect.Value, error) {
	ftyp := field.Type
	if ftyp.Kind() != reflect.Func {
		return reflect.Value{}, xerrors.New("handler field not a func")
	}

	rm := &rpcMethod{
		method:  fmt.Sprintf("%s.%s", namespace, field.Name),
		idgen:   idgen,
		cli:     cli,
		refFunc: ftyp,
	}

	rm.outValIdx, rm.outErrIdx, rm.outNum = processFuncOut(ftyp)

	rm.inHasCtx = ftyp.NumIn() > 0 && ftyp.In(0) == contextType
	rm.returnChan = rm.outValIdx != -1 && ftyp.Out(rm.outValIdx).Kind() == reflect.Chan

	var mods []CallOptionModifier
	_, failfast := field.Tag.Lookup("failfast")
	mods = append(mods, FailFast(failfast))

	rm.callOptMods = mods

	log.Debugf("rpc method constructed %#v", rm)
	return reflect.MakeFunc(ftyp, rm.call), nil
}

// parsed func
type rpcMethod struct {
	method string
	idgen  idGenerator
	cli    Client

	refFunc reflect.Type

	inHasCtx bool

	outNum    int
	outValIdx int
	outErrIdx int

	returnChan  bool
	callOptMods []CallOptionModifier
}

func (rm *rpcMethod) call(args []reflect.Value) []reflect.Value {
	id := int64(rm.idgen.next())
	meta := map[string]string{}

	var rawCtx context.Context
	var span *trace.Span

	if rm.inHasCtx {
		rawCtx = args[0].Interface().(context.Context)
		rawCtx, span = trace.StartSpan(rawCtx, "api.call")
		defer span.End()

		span.AddAttributes(trace.StringAttribute("method", rm.method))

		meta["SpanContext"] = base64.StdEncoding.EncodeToString(
			propagation.Binary(span.SpanContext()))

		args = args[1:]

	} else {
		rawCtx = context.Background()
	}

	params := make([]param, len(args))
	for i := range args {
		params[i] = param{
			v: args[i],
		}
	}

	ctx, cancel := context.WithCancel(rawCtx)

	callOpt := rm.cli.CallOption()

	for _, opt := range rm.callOptMods {
		opt(&callOpt)
	}

	rpcCtx := &rpcContext{
		id:     id,
		ctx:    ctx,
		cancel: cancel,
		req: frame{
			Jsonrpc: "2.0",
			ID:      &id,
			Method:  rm.method,
			Params:  params,
		},
		isChan: rm.returnChan,
		opt:    callOpt,
		resCh:  make(chan dataOrErr, 1),
		logger: log.With("method", rm.method, "reqid", id),
	}

	rm.cli.handleCall(rpcCtx)

	select {
	case <-ctx.Done():
		cancel()
		return rm.processError(xerrors.Errorf("%w: %s", ErrContextDone, ctx.Err()))

	case de, ok := <-rpcCtx.resCh:
		if !ok {
			cancel()
			return rm.processError(ErrNoResponse)
		}

		if de.err != nil {
			cancel()
			return rm.processError(de.err)
		}

		if !rpcCtx.isChan {
			return rm.processSingleCall(rpcCtx, de)
		}

		return rm.processOutChan(rm.cli, rpcCtx)
	}
}

func (rm *rpcMethod) processError(err error) []reflect.Value {
	out := make([]reflect.Value, rm.outNum)

	if rm.outValIdx != -1 {
		out[rm.outValIdx] = reflect.New(rm.refFunc.Out(rm.outValIdx)).Elem()
	}

	if rm.outErrIdx != -1 {
		out[rm.outErrIdx] = reflect.New(errorType).Elem()
		out[rm.outErrIdx].Set(reflect.ValueOf(err))
	}

	return out
}

func (rm *rpcMethod) processResponse(val reflect.Value) []reflect.Value {
	out := make([]reflect.Value, rm.outNum)

	if rm.outValIdx != -1 {
		out[rm.outValIdx] = val
	}

	if rm.outErrIdx != -1 {
		out[rm.outErrIdx] = reflect.New(errorType).Elem()
	}

	return out
}

func (rm *rpcMethod) processSingleCall(rctx *rpcContext, de dataOrErr) []reflect.Value {
	defer rctx.cancel()

	if rm.outValIdx == -1 {
		return rm.processResponse(nullValue)
	}

	val := reflect.New(rm.refFunc.Out(rm.outValIdx))

	rctx.logger.Debugf("rpc result type=%v", rm.refFunc.Out(rm.outValIdx))
	if err := de.extractJSON(val.Interface(), false); err != nil {
		return rm.processError(err)
	}

	return rm.processResponse(val)
}

func (rm *rpcMethod) processOutChan(cli Client, rctx *rpcContext) []reflect.Value {
	rctx.logger.Debug("chan handshake made")

	elemType := rm.refFunc.Out(rm.outValIdx).Elem()
	stream, err := newRPCStream(cli, rctx, elemType)
	if err != nil {
		rctx.cancel()
		return rm.processError(err)
	}

	go stream.start()

	return rm.processResponse(stream.outCh.Convert(rm.refFunc.Out(rm.outValIdx)))
}

func newRPCStream(cli Client, rctx *rpcContext, elemType reflect.Type) (*rpcStream, error) {
	chType := reflect.ChanOf(reflect.BothDir, elemType)
	outCh := reflect.MakeChan(chType, 1)

	return &rpcStream{
		cli:      cli,
		rctx:     rctx,
		elemType: elemType,
		outCh:    outCh,
	}, nil
}

type rpcStream struct {
	cli      Client
	rctx     *rpcContext
	elemType reflect.Type
	outCh    reflect.Value
}

func (rs *rpcStream) start() {
	rs.rctx.logger.Debug("rpc stream loop start")

	defer func() {
		rs.outCh.Close()
		rs.rctx.cancel()
		rs.cli.cancel(rs.rctx)
		rs.rctx.logger.Debug("rpc stream loop stop")
	}()

	selCases := [2]reflect.SelectCase{}
	selCases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(rs.rctx.ctx.Done()),
	}

	for {
		select {
		case <-rs.rctx.ctx.Done():
			return

		case de, ok := <-rs.rctx.resCh:
			if !ok {
				return
			}

			if de.err != nil {
				if de.err == ErrRequestFinished {
					rs.rctx.logger.Debugf("received terminate err")
				} else {
					rs.rctx.logger.Warnf("received err: %s", de.err)
				}

				return
			}

			val := reflect.New(rs.elemType)
			if err := de.extractJSON(val.Interface(), true); err != nil {
				rs.rctx.logger.Warnf("unable to extract json data from incoming: %s", err)
				return
			}

			selCases[1] = reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: rs.outCh,
				Send: val.Elem(),
			}

			choose, _, _ := reflect.Select(selCases[:])
			if choose == 0 {
				return
			}
		}
	}
}
