package rpcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var log = logging.Logger("rpc2")

const (
	reconnectMinInterval = time.Second
	reconnectMaxInterval = 64 * time.Second
)

// NewMergeClient parses struct and set it's field with method
func NewMergeClient(addr string, namespace string, outs []interface{}, header http.Header) (ClientCloser, error) {
	connector := NewWebsocketConnector(addr, header)
	client, closer, err := NewSimpleClient(connector, nil)
	if err != nil {
		return nil, err
	}

	idgen := &simpleIDGen{}

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
			fn, err := newRPCMethod(typ.Field(i), client, idgen, namespace)
			if err != nil {
				return nil, err
			}

			val.Elem().Field(i).Set(fn)
		}
	}

	return closer, nil
}

var _ Client = (*simpleClient)(nil)

// Client represents a rpc Client, with all required policies inside
type Client interface {
	CallOption() CallOption

	handleCall(*rpcContext)
	cancel(*rpcContext)
}

type chanMapIdentifier struct {
	connID uint64
	chanID uint64
}

// ClientCloser controls
type ClientCloser = func()

// NewSimpleClient connects and returns a Client
func NewSimpleClient(connector Connector, opt *ClientOption) (Client, ClientCloser, error) {
	if opt == nil {
		opt = &defaultClientOption
	}

	ctx := context.Background()

	dialCtx := ctx
	if opt.dialTimeout > 0 {
		var dialCancel context.CancelFunc

		dialCtx, dialCancel = context.WithTimeout(ctx, opt.dialTimeout)
		defer dialCancel()

	}

	firstConn, err := connector.Dial(dialCtx)
	if err != nil {
		return nil, nil, xerrors.Errorf("unable to connect: %w", err)
	}

	running, cancel := context.WithCancel(ctx)

	cli := &simpleClient{
		Connector: connector,

		opt: *opt,

		handling:   map[int64]*rpcContext{},
		chanReqMap: map[chanMapIdentifier]int64{},
		reqChanMap: map[int64]map[chanMapIdentifier]struct{}{},
		firstConn:  firstConn,
		reqIn:      make(chan *rpcContext),
		cancelIn:   make(chan *rpcContext),

		shouldReconnect: make(chan struct{}, 1),
		reconnected:     make(chan RawConn),

		running: running,
	}

	go cli.run()

	return cli, cancel, nil
}

type simpleClient struct {
	Connector

	opt ClientOption

	handling   map[int64]*rpcContext
	chanReqMap map[chanMapIdentifier]int64
	reqChanMap map[int64]map[chanMapIdentifier]struct{}

	firstConn RawConn

	reqIn    chan *rpcContext
	cancelIn chan *rpcContext

	shouldReconnect chan struct{}
	reconnected     chan RawConn

	running context.Context
}

func (sc *simpleClient) CallOption() CallOption {
	return sc.opt.call
}

func (sc *simpleClient) handleCall(rctx *rpcContext) {
	go func() {
		select {
		case <-sc.running.Done():
			rctx.finish(nil, ErrClientClosed)
			return

		case <-rctx.ctx.Done():
			rctx.finish(nil, rctx.ctx.Err())
			return

		case sc.reqIn <- rctx:

		}
	}()
}

func (sc *simpleClient) cancel(rctx *rpcContext) {
	go func() {
		select {
		case <-sc.running.Done():
			rctx.finish(nil, ErrClientClosed)
			return

		case <-rctx.ctx.Done():
			rctx.finish(nil, rctx.ctx.Err())
			return

		case sc.cancelIn <- rctx:

		}
	}()
}

func (sc *simpleClient) run() {
	go sc.startReconnectLoop()

	conn := sc.firstConn
	incomingCh, connCancel := sc.startRawConn(sc.running, conn)

	defer func() {
		sc.stop()
		if connCancel != nil {
			connCancel()
		}
	}()

	var cleanupBadConn = func() {
		if conn == nil {
			return
		}

		log.Debug("cleanup due to bad connectiosn")

		connCancel()

		connCancel = nil
		conn = nil

		// TODO: shall we consume remaining items in the incoming ch?
		incomingCh = nil

		sc.finishFailFastRequests(sc.running)
		sc.triggerReconnect(sc.running)
	}

CLIENT_LOOP:
	for {
		select {
		case <-sc.running.Done():
			return

		// incoming msg from underlying connection
		case msgOrErr, ok := <-incomingCh:
			if !ok {
				cleanupBadConn()
				continue CLIENT_LOOP
			}

			if msgOrErr.err != nil {
				log.Warnf("get error from underlying incoming message loop: %s", msgOrErr.err)

				if isWsConnectionErr(msgOrErr.err) {
					cleanupBadConn()
				}

				continue CLIENT_LOOP
			}

			sc.handleFrame(sc.running, msgOrErr.connID, msgOrErr.frame)

		// new rpc request
		case newReq := <-sc.reqIn:
			select {
			case <-newReq.ctx.Done():
				newReq.logger.Debug("request already canceled")
				newReq.finish(nil, nil)
				continue CLIENT_LOOP

			default:

			}

			err := conn.sendFrame(newReq.ctx, newReq.req)
			if err != nil {
				isConnErr := isWsConnectionErr(err)
				if isConnErr {
					cleanupBadConn()
				}

				// hold all non-failfast request here if we have no available ws conn
				if !isConnErr || newReq.opt.failfast {
					newReq.finish(nil, err)
					continue CLIENT_LOOP
				}
			}

			sc.registerRequest(newReq)

		// cancel given rpc request
		case canceled := <-sc.cancelIn:
			rctx, ok := sc.handling[canceled.id]
			if !ok {
				continue CLIENT_LOOP
			}

			// send wsCancel for continuous requesst
			if rctx.isChan {
				if err := conn.sendFrame(sc.running, frame{
					Jsonrpc: "2.0",
					Method:  wsCancel,
					Params:  []param{{v: reflect.ValueOf(rctx.id)}},
				}); err != nil {
					rctx.logger.Warnf("unable to send wsCancel: %s", err)
				}
			}

			sc.onRequestErr(rctx.id, nil, ErrRequestFinished)

		case newConn := <-sc.reconnected:
			incomingCh, connCancel = sc.startRawConn(sc.running, newConn)
			conn = newConn

			// resend requests for non-failfast calls
			for reqID := range sc.handling {
				rctx := sc.handling[reqID]
				if err := conn.sendFrame(rctx.ctx, rctx.req); err != nil {
					if isWsConnectionErr(err) {
						cleanupBadConn()
						break
					}

					rctx.logger.Debug("request retried but failed")
					sc.onRequestErr(reqID, nil, err)
				}
			}
		}
	}
}

func (sc *simpleClient) startReconnectLoop() {
	var reconnTimer <-chan time.Time
	interval := reconnectMinInterval

RE_LOOP:
	for {
		select {
		case <-sc.running.Done():
			return

		case <-sc.shouldReconnect:
			// only setup the timer if there is no previous reonnecting attemption
			if reconnTimer == nil {
				log.Debugf("websocket will auto-recoonect after %s", interval)
				reconnTimer = time.After(interval)
			}

		case <-reconnTimer:
			conn, err := sc.Dial(sc.running)
			if err != nil {
				interval *= 2
				if interval > reconnectMaxInterval {
					interval = reconnectMaxInterval
				}

				reconnTimer = time.After(interval)

				log.Warnf("unable to establish a websocket connection: %s, will recoonect after %s", err, interval)

				continue RE_LOOP
			}

			select {
			case <-sc.running.Done():
				return

			case sc.reconnected <- conn:
				// reset timer and interval here
				interval = reconnectMinInterval
				reconnTimer = nil
			}
		}
	}
}

func (sc *simpleClient) startRawConn(ctx context.Context, conn RawConn) (<-chan rawMsgOrErr, context.CancelFunc) {
	connCtx, connCancel := context.WithCancel(ctx)

	inCh := conn.startIncomingLoop(connCtx)

	return inCh, connCancel
}

func (sc *simpleClient) registerRequest(rctx *rpcContext) {
	sc.handling[rctx.id] = rctx
}

func (sc *simpleClient) unregisterRequest(reqID int64) {
	if _, ok := sc.handling[reqID]; ok {
		delete(sc.handling, reqID)
	}

	if m, ok := sc.reqChanMap[reqID]; ok {
		delete(sc.reqChanMap, reqID)

		for mapID := range m {
			if _, ok := sc.chanReqMap[mapID]; ok {
				delete(sc.chanReqMap, mapID)
			}
		}
	}
}

func (sc *simpleClient) registerChanMap(connID, chanID uint64, reqID int64) {
	mid := chanMapIdentifier{
		connID: connID,
		chanID: chanID,
	}

	sc.chanReqMap[mid] = reqID
	if sc.reqChanMap[reqID] == nil {
		sc.reqChanMap[reqID] = map[chanMapIdentifier]struct{}{}
	}

	sc.reqChanMap[reqID][mid] = struct{}{}
}

func (sc *simpleClient) findRPCContext(connID, chanID uint64) (*rpcContext, bool) {
	mid := chanMapIdentifier{
		connID: connID,
		chanID: chanID,
	}

	reqID, ok := sc.chanReqMap[mid]
	if !ok {
		return nil, false
	}

	rctx, ok := sc.handling[reqID]
	if !ok {
		delete(sc.chanReqMap, mid)
	}

	return rctx, ok
}

func (sc *simpleClient) handleFrame(ctx context.Context, connID uint64, f frame) {
	switch f.Method {
	case "":
		sc.handleResponseFrame(ctx, connID, f)

	case chValue:
		sc.handleChanMessageFrame(ctx, connID, f)

	case chClose:
		sc.handleChanCloseFrame(ctx, connID, f)

	default:
		log.Warnf("unexpected frame method for clien-side %s", f.Method)
	}
}

func (sc *simpleClient) handleResponseFrame(ctx context.Context, connID uint64, f frame) {
	if f.ID == nil {
		log.Warn("got response frame without request id")
		return
	}

	reqID := *f.ID

	rctx, ok := sc.handling[reqID]
	if !ok {
		log.Warnf("got response frame with non-exist requesst id %d", *f.ID)
		return
	}

	if f.Error != nil {
		sc.onRequestErr(reqID, &connID, f.Error)
		return
	}

	if !rctx.isChan {
		rctx.send(dataOrErr{
			connID: &connID,

			// TODO: use pooled Buffer or Reader here
			r: ioutil.NopCloser(bytes.NewReader(f.Result)),
		})

		sc.unregisterRequest(reqID)
		return
	}

	handshaked := false
	var chanID uint64
	if f.Result != nil {
		if err := json.Unmarshal(f.Result, &chanID); err != nil {
			rctx.logger.Warnf("unable to parse chan id: %s", err)
		} else {
			handshaked = true
		}
	}

	if !handshaked {
		sc.onRequestErr(reqID, &connID, ErrBadHandshake)
		return
	}

	rctx.logger.Debugf("assigned to conn %d chan %d", connID, chanID)
	sc.registerChanMap(connID, chanID, reqID)

	// we only send mapping frame on init
	if !rctx.streaming {
		rctx.send(dataOrErr{
			connID: &connID,
			err:    nil,
			r:      nil,
		})

		rctx.streaming = true
	}
}

func (sc *simpleClient) handleChanMessageFrame(ctx context.Context, connID uint64, f frame) {
	if len(f.Params) != 2 {
		log.Warnf("expected 2 params in %s frame, got %d", chValue, len(f.Params))
		return
	}

	var chanID uint64
	if err := json.Unmarshal(f.Params[0].data, &chanID); err != nil {
		log.Errorf("unable to unmarshal channel id in %s: %s", chValue, err)
		return
	}

	rctx, ok := sc.findRPCContext(connID, chanID)
	if !ok {
		log.Warnf("unable to find rpc context for conn %d chan %d", connID, chanID)
		return
	}

	rctx.send(dataOrErr{
		connID: &connID,
		r:      ioutil.NopCloser(bytes.NewReader(f.Params[1].data)),
	})
}

func (sc *simpleClient) handleChanCloseFrame(ctx context.Context, connID uint64, f frame) {
	if len(f.Params) != 1 {
		log.Warnf("expected 1 params in %s frame, got %d", chClose, len(f.Params))
		return
	}

	var chanID uint64
	if err := json.Unmarshal(f.Params[0].data, &chanID); err != nil {
		log.Errorf("unable to unmarshal channel id in %s: %s", chClose, err)
		return
	}

	rctx, ok := sc.findRPCContext(connID, chanID)
	if !ok {
		log.Warnf("unable to find rpc context for conn %d chan %d", connID, chanID)
		return
	}

	rctx.finish(&connID, ErrRequestFinished)
}

func (sc *simpleClient) finishFailFastRequests(ctx context.Context) {
	for reqID := range sc.handling {
		rctx := sc.handling[reqID]
		if rctx.opt.failfast {
			rctx.logger.Debug("finish request due to failfast option")
			sc.onRequestErr(reqID, nil, ErrNoAvailableConnection)
		}
	}
}

func (sc *simpleClient) triggerReconnect(ctx context.Context) {
	select {
	case <-ctx.Done():

	case sc.shouldReconnect <- struct{}{}:

	}
}

func (sc *simpleClient) stop() {
	for reqID := range sc.handling {
		sc.onRequestErr(reqID, nil, ErrClientClosed)
	}
}

func (sc *simpleClient) onRequestErr(reqID int64, connID *uint64, err error) {
	rctx, ok := sc.handling[reqID]
	if !ok {
		return
	}

	rctx.finish(connID, err)
	sc.unregisterRequest(reqID)
}
