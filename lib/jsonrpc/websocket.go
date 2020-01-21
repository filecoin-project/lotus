package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"golang.org/x/xerrors"
)

const wsCancel = "xrpc.cancel"
const chValue = "xrpc.ch.val"
const chClose = "xrpc.ch.close"

type frame struct {
	// common
	Jsonrpc string            `json:"jsonrpc"`
	ID      *int64            `json:"id,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`

	// request
	Method string  `json:"method,omitempty"`
	Params []param `json:"params,omitempty"`

	// response
	Result json.RawMessage `json:"result,omitempty"`
	Error  *respError      `json:"error,omitempty"`
}

type outChanReg struct {
	reqID int64

	chID uint64
	ch   reflect.Value
}

type wsConn struct {
	// outside params
	conn     *websocket.Conn
	handler  handlers
	requests <-chan clientRequest
	stop     <-chan struct{}
	exiting  chan struct{}

	// incoming messages
	incoming    chan io.Reader
	incomingErr error

	// outgoing messages
	writeLk sync.Mutex

	// ////
	// Client related

	// inflight are requests we've sent to the remote
	inflight map[int64]clientRequest

	// chanHandlers is a map of client-side channel handlers
	chanHandlers map[uint64]func(m []byte, ok bool)

	// ////
	// Server related

	// handling are the calls we handle
	handling   map[int64]context.CancelFunc
	handlingLk sync.Mutex

	spawnOutChanHandlerOnce sync.Once

	// chanCtr is a counter used for identifying output channels on the server side
	chanCtr uint64

	registerCh chan outChanReg
}

//                         //
// WebSocket Message utils //
//                         //

// nextMessage wait for one message and puts it to the incoming channel
func (c *wsConn) nextMessage() {
	msgType, r, err := c.conn.NextReader()
	if err != nil {
		c.incomingErr = err
		close(c.incoming)
		return
	}
	if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
		c.incomingErr = errors.New("unsupported message type")
		close(c.incoming)
		return
	}
	c.incoming <- r
}

// nextWriter waits for writeLk and invokes the cb callback with WS message
// writer when the lock is acquired
func (c *wsConn) nextWriter(cb func(io.Writer)) {
	c.writeLk.Lock()
	defer c.writeLk.Unlock()

	wcl, err := c.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		log.Error("handle me:", err)
		return
	}

	cb(wcl)

	if err := wcl.Close(); err != nil {
		log.Error("handle me:", err)
		return
	}
}

func (c *wsConn) sendRequest(req request) {
	c.writeLk.Lock()
	if err := c.conn.WriteJSON(req); err != nil {
		log.Error("handle me:", err)
		c.writeLk.Unlock()
		return
	}
	c.writeLk.Unlock()
}

//                 //
// Output channels //
//                 //

// handleOutChans handles channel communication on the server side
// (forwards channel messages to client)
func (c *wsConn) handleOutChans() {
	regV := reflect.ValueOf(c.registerCh)
	exitV := reflect.ValueOf(c.exiting)

	cases := []reflect.SelectCase{
		{ // registration chan always 0
			Dir:  reflect.SelectRecv,
			Chan: regV,
		},
		{ // exit chan always 1
			Dir:  reflect.SelectRecv,
			Chan: exitV,
		},
	}
	internal := len(cases)
	var caseToID []uint64

	for {
		chosen, val, ok := reflect.Select(cases)

		switch chosen {
		case 0: // registration channel
			if !ok {
				// control channel closed - signals closed connection
				// This shouldn't happen, instead the exiting channel should get closed
				log.Warn("control channel closed")
				return
			}

			registration := val.Interface().(outChanReg)

			caseToID = append(caseToID, registration.chID)
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: registration.ch,
			})

			c.nextWriter(func(w io.Writer) {
				resp := &response{
					Jsonrpc: "2.0",
					ID:      registration.reqID,
					Result:  registration.chID,
				}

				if err := json.NewEncoder(w).Encode(resp); err != nil {
					log.Error(err)
					return
				}
			})

			continue
		case 1: // exiting channel
			if !ok {
				// exiting channel closed - signals closed connection
				//
				// We're not closing any channels as we're on receiving end.
				// Also, context cancellation below should take care of any running
				// requests
				return
			}
			log.Warn("exiting channel received a message")
			continue
		}

		if !ok {
			// Output channel closed, cleanup, and tell remote that this happened

			n := len(cases) - 1
			if n > 0 {
				cases[chosen] = cases[n]
				caseToID[chosen-internal] = caseToID[n-internal]
			}

			id := caseToID[chosen-internal]
			cases = cases[:n]
			caseToID = caseToID[:n-internal]

			c.sendRequest(request{
				Jsonrpc: "2.0",
				ID:      nil, // notification
				Method:  chClose,
				Params:  []param{{v: reflect.ValueOf(id)}},
			})
			continue
		}

		// forward message
		c.sendRequest(request{
			Jsonrpc: "2.0",
			ID:      nil, // notification
			Method:  chValue,
			Params:  []param{{v: reflect.ValueOf(caseToID[chosen-internal])}, {v: val}},
		})
	}
}

// handleChanOut registers output channel for forwarding to client
func (c *wsConn) handleChanOut(ch reflect.Value, req int64) error {
	c.spawnOutChanHandlerOnce.Do(func() {
		go c.handleOutChans()
	})
	id := atomic.AddUint64(&c.chanCtr, 1)

	select {
	case c.registerCh <- outChanReg{
		reqID: req,

		chID: id,
		ch:   ch,
	}:
		return nil
	case <-c.exiting:
		return xerrors.New("connection closing")
	}
}

//                          //
// Context.Done propagation //
//                          //

// handleCtxAsync handles context lifetimes for client
// TODO: this should be aware of events going through chanHandlers, and quit
//  when the related channel is closed.
//  This should also probably be a single goroutine,
//  Note that not doing this should be fine for now as long as we are using
//  contexts correctly (cancelling when async functions are no longer is use)
func (c *wsConn) handleCtxAsync(actx context.Context, id int64) {
	<-actx.Done()

	c.sendRequest(request{
		Jsonrpc: "2.0",
		Method:  wsCancel,
		Params:  []param{{v: reflect.ValueOf(id)}},
	})
}

// cancelCtx is a built-in rpc which handles context cancellation over rpc
func (c *wsConn) cancelCtx(req frame) {
	if req.ID != nil {
		log.Warnf("%s call with ID set, won't respond", wsCancel)
	}

	var id int64
	if err := json.Unmarshal(req.Params[0].data, &id); err != nil {
		log.Error("handle me:", err)
		return
	}

	c.handlingLk.Lock()
	defer c.handlingLk.Unlock()

	cf, ok := c.handling[id]
	if ok {
		cf()
	}
}

//                     //
// Main Handling logic //
//                     //

func (c *wsConn) handleChanMessage(frame frame) {
	var chid uint64
	if err := json.Unmarshal(frame.Params[0].data, &chid); err != nil {
		log.Error("failed to unmarshal channel id in xrpc.ch.val: %s", err)
		return
	}

	hnd, ok := c.chanHandlers[chid]
	if !ok {
		log.Errorf("xrpc.ch.val: handler %d not found", chid)
		return
	}

	hnd(frame.Params[1].data, true)
}

func (c *wsConn) handleChanClose(frame frame) {
	var chid uint64
	if err := json.Unmarshal(frame.Params[0].data, &chid); err != nil {
		log.Error("failed to unmarshal channel id in xrpc.ch.val: %s", err)
		return
	}

	hnd, ok := c.chanHandlers[chid]
	if !ok {
		log.Errorf("xrpc.ch.val: handler %d not found", chid)
		return
	}

	delete(c.chanHandlers, chid)

	hnd(nil, false)
}

func (c *wsConn) handleResponse(frame frame) {
	req, ok := c.inflight[*frame.ID]
	if !ok {
		log.Error("client got unknown ID in response")
		return
	}

	if req.retCh != nil && frame.Result != nil {
		// output is channel
		var chid uint64
		if err := json.Unmarshal(frame.Result, &chid); err != nil {
			log.Errorf("failed to unmarshal channel id response: %s, data '%s'", err, string(frame.Result))
			return
		}

		var chanCtx context.Context
		chanCtx, c.chanHandlers[chid] = req.retCh()
		go c.handleCtxAsync(chanCtx, *frame.ID)
	}

	req.ready <- clientResponse{
		Jsonrpc: frame.Jsonrpc,
		Result:  frame.Result,
		ID:      *frame.ID,
		Error:   frame.Error,
	}
	delete(c.inflight, *frame.ID)
}

func (c *wsConn) handleCall(ctx context.Context, frame frame) {
	req := request{
		Jsonrpc: frame.Jsonrpc,
		ID:      frame.ID,
		Meta:    frame.Meta,
		Method:  frame.Method,
		Params:  frame.Params,
	}

	ctx, cancel := context.WithCancel(ctx)

	nextWriter := func(cb func(io.Writer)) {
		cb(ioutil.Discard)
	}
	done := func(keepCtx bool) {
		if !keepCtx {
			cancel()
		}
	}
	if frame.ID != nil {
		nextWriter = c.nextWriter

		c.handlingLk.Lock()
		c.handling[*frame.ID] = cancel
		c.handlingLk.Unlock()

		done = func(keepctx bool) {
			c.handlingLk.Lock()
			defer c.handlingLk.Unlock()

			if !keepctx {
				cancel()
				delete(c.handling, *frame.ID)
			}
		}
	}

	go c.handler.handle(ctx, req, nextWriter, rpcError, done, c.handleChanOut)
}

// handleFrame handles all incoming messages (calls and responses)
func (c *wsConn) handleFrame(ctx context.Context, frame frame) {
	// Get message type by method name:
	// "" - response
	// "xrpc.*" - builtin
	// anything else - incoming remote call
	switch frame.Method {
	case "": // Response to our call
		c.handleResponse(frame)
	case wsCancel:
		c.cancelCtx(frame)
	case chValue:
		c.handleChanMessage(frame)
	case chClose:
		c.handleChanClose(frame)
	default: // Remote call
		c.handleCall(ctx, frame)
	}
}

func (c *wsConn) handleWsConn(ctx context.Context) {
	c.incoming = make(chan io.Reader)
	c.inflight = map[int64]clientRequest{}
	c.handling = map[int64]context.CancelFunc{}
	c.chanHandlers = map[uint64]func(m []byte, ok bool){}

	c.registerCh = make(chan outChanReg)
	defer close(c.exiting)

	// ////

	// on close, make sure to return from all pending calls, and cancel context
	//  on all calls we handle
	defer func() {
		for id, req := range c.inflight {
			req.ready <- clientResponse{
				Jsonrpc: "2.0",
				ID:      id,
				Error: &respError{
					Message: "handler: websocket connection closed",
				},
			}

			c.handlingLk.Lock()
			for _, cancel := range c.handling {
				cancel()
			}
			c.handlingLk.Unlock()
		}
	}()

	// wait for the first message
	go c.nextMessage()

	for {
		select {
		case r, ok := <-c.incoming:
			if !ok {
				if c.incomingErr != nil {
					if !websocket.IsCloseError(c.incomingErr, websocket.CloseNormalClosure) {
						log.Debugw("websocket error", "error", c.incomingErr)
					}
				}
				return // remote closed
			}

			// debug util - dump all messages to stderr
			// r = io.TeeReader(r, os.Stderr)

			var frame frame
			if err := json.NewDecoder(r).Decode(&frame); err != nil {
				log.Error("handle me:", err)
				return
			}

			c.handleFrame(ctx, frame)
			go c.nextMessage()
		case req := <-c.requests:
			if req.req.ID != nil {
				c.inflight[*req.req.ID] = req
			}
			c.sendRequest(req.req)
		case <-c.stop:
			c.writeLk.Lock()
			cmsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			if err := c.conn.WriteMessage(websocket.CloseMessage, cmsg); err != nil {
				log.Warn("failed to write close message: ", err)
			}
			if err := c.conn.Close(); err != nil {
				log.Warnw("websocket close error", "error", err)
			}
			c.writeLk.Unlock()
			return
		}
	}
}
