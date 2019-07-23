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
)

const wsCancel = "xrpc.cancel"
const chValue = "xrpc.ch.val"
const chClose = "xrpc.ch.close"

type frame struct {
	// common
	Jsonrpc string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`

	// request
	Method string  `json:"method,omitempty"`
	Params []param `json:"params,omitempty"`

	// response
	Result json.RawMessage `json:"result,omitempty"`
	Error  *respError      `json:"error,omitempty"`
}

type outChanReg struct {
	id uint64
	ch reflect.Value
}

type wsConn struct {
	// outside params
	conn     *websocket.Conn
	handler  handlers
	requests <-chan clientRequest
	stop     <-chan struct{}

	// incoming messages
	incoming    chan io.Reader
	incomingErr error

	// outgoing messages
	writeLk sync.Mutex

	// ////
	// Client related

	// inflight are requests we've sent to the remote
	inflight map[int64]clientRequest

	// ////
	// Server related

	// handling are the calls we handle
	handling   map[int64]context.CancelFunc
	handlingLk sync.Mutex

	spawnOutChanHandlerOnce sync.Once

	// chanCtr is a counter used for identifying output channels on the server side
	chanCtr uint64
}

func (c *wsConn) handleWsConn(ctx context.Context) {
	c.incoming = make(chan io.Reader)
	c.inflight = map[int64]clientRequest{}
	c.handling = map[int64]context.CancelFunc{}

	var registerCh = make(chan outChanReg)
	defer close(registerCh)

	// chanHandlers is a map of client-side channel handlers
	chanHandlers := map[uint64]func(m []byte, ok bool){}

	// nextMessage wait for one message and puts it to the incoming channel
	nextMessage := func() {
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
	nextWriter := func(cb func(io.Writer)) {
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

	sendReq := func(req request) {
		c.writeLk.Lock()
		if err := c.conn.WriteJSON(req); err != nil {
			log.Error("handle me:", err)
			c.writeLk.Unlock()
			return
		}
		c.writeLk.Unlock()
	}

	// ////
	// Subscriptions (func() <-chan Typ - like methods)

	// handleOutChans handles channel communication on the server side
	// (forwards channel messages to client)
	handleOutChans := func() {
		regV := reflect.ValueOf(registerCh)

		cases := []reflect.SelectCase{
			{ // registration chan always 0
				Dir:  reflect.SelectRecv,
				Chan: regV,
			},
		}
		var caseToID []uint64

		for {
			chosen, val, ok := reflect.Select(cases)

			if chosen == 0 { // control channel
				if !ok {
					// control channel closed - signals closed connection
					//
					// We're not closing any channels as we're on receiving end.
					// Also, context cancellation below should take care of any running
					// requests
					return
				}

				registration := val.Interface().(outChanReg)

				caseToID = append(caseToID, registration.id)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: registration.ch,
				})

				continue
			}

			if !ok {
				// Output channel closed, cleanup, and tell remote that this happened

				n := len(caseToID)
				if n > 0 {
					cases[chosen] = cases[n]
					caseToID[chosen-1] = caseToID[n-1]
				}

				id := caseToID[chosen-1]
				cases = cases[:n]
				caseToID = caseToID[:n-1]

				sendReq(request{
					Jsonrpc: "2.0",
					ID:      nil, // notification
					Method:  chClose,
					Params:  []param{{v: reflect.ValueOf(id)}},
				})
				continue
			}

			// forward message
			sendReq(request{
				Jsonrpc: "2.0",
				ID:      nil, // notification
				Method:  chValue,
				Params:  []param{{v: reflect.ValueOf(caseToID[chosen-1])}, {v: val}},
			})
		}
	}

	// handleChanOut registers output channel for forwarding to client
	handleChanOut := func(ch reflect.Value) interface{} {
		c.spawnOutChanHandlerOnce.Do(func() {
			go handleOutChans()
		})
		id := atomic.AddUint64(&c.chanCtr, 1)

		registerCh <- outChanReg{
			id: id,
			ch: ch,
		}

		return id
	}

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

	// handleCtxAsync handles context lifetimes for client
	// TODO: this should be aware of events going through chanHandlers, and quit
	//  when the related channel is closed.
	//  This should also probably be a single goroutine,
	//  Note that not doing this should be fine for now as long as we are using
	//  contexts correctly (cancelling when async functions are no longer is use)
	handleCtxAsync := func(actx context.Context, id int64) {
		<-actx.Done()

		sendReq(request{
			Jsonrpc: "2.0",
			Method:  wsCancel,
			Params:  []param{{v: reflect.ValueOf(id)}},
		})
	}

	// cancelCtx is a built-in rpc which handles context cancellation over rpc
	cancelCtx := func(req frame) {
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

	// wait for the first message
	go nextMessage()
	var msgConsumed bool

	for {
		if msgConsumed {
			msgConsumed = false
			go nextMessage()
		}

		select {
		case r, ok := <-c.incoming:
			if !ok {
				if c.incomingErr != nil {
					log.Debugf("websocket error", "error", c.incomingErr)
				}
				return // remote closed
			}
			msgConsumed = true

			// debug util - dump all messages to stderr
			// r = io.TeeReader(r, os.Stderr)

			var frame frame
			if err := json.NewDecoder(r).Decode(&frame); err != nil {
				log.Error("handle me:", err)
				return
			}

			// Get message type by method name:
			// "" - response
			// "xrpc.*" - builtin
			// anything else - incoming remote call
			switch frame.Method {
			case "": // Response to our call
				req, ok := c.inflight[*frame.ID]
				if !ok {
					log.Error("client got unknown ID in response")
					continue
				}

				if req.retCh != nil && frame.Result != nil {
					// output is channel
					var chid uint64
					if err := json.Unmarshal(frame.Result, &chid); err != nil {
						log.Errorf("failed to unmarshal channel id response: %s, data '%s'", err, string(frame.Result))
						continue
					}

					var chanCtx context.Context
					chanCtx, chanHandlers[chid] = req.retCh()
					go handleCtxAsync(chanCtx, *frame.ID)
				}

				req.ready <- clientResponse{
					Jsonrpc: frame.Jsonrpc,
					Result:  frame.Result,
					ID:      *frame.ID,
					Error:   frame.Error,
				}
				delete(c.inflight, *frame.ID)
			case wsCancel:
				cancelCtx(frame)
			case chValue:
				var chid uint64
				if err := json.Unmarshal(frame.Params[0].data, &chid); err != nil {
					log.Error("failed to unmarshal channel id in xrpc.ch.val: %s", err)
					continue
				}

				hnd, ok := chanHandlers[chid]
				if !ok {
					log.Errorf("xrpc.ch.val: handler %d not found", chid)
					continue
				}

				hnd(frame.Params[1].data, true)
			case chClose:
				var chid uint64
				if err := json.Unmarshal(frame.Params[0].data, &chid); err != nil {
					log.Error("failed to unmarshal channel id in xrpc.ch.val: %s", err)
					continue
				}

				hnd, ok := chanHandlers[chid]
				if !ok {
					log.Errorf("xrpc.ch.val: handler %d not found", chid)
					continue
				}

				delete(chanHandlers, chid)

				hnd(nil, false)
			default: // Remote call
				req := request{
					Jsonrpc: frame.Jsonrpc,
					ID:      frame.ID,
					Method:  frame.Method,
					Params:  frame.Params,
				}

				ctx, cf := context.WithCancel(ctx)

				nw := func(cb func(io.Writer)) {
					cb(ioutil.Discard)
				}
				done := func(keepctx bool) {
					if !keepctx {
						cf()
					}
				}
				if frame.ID != nil {
					nw = nextWriter

					c.handlingLk.Lock()
					c.handling[*frame.ID] = cf
					c.handlingLk.Unlock()

					done = func(keepctx bool) {
						c.handlingLk.Lock()
						defer c.handlingLk.Unlock()

						if !keepctx {
							cf()
							delete(c.handling, *frame.ID)
						}
					}
				}

				go c.handler.handle(ctx, req, nw, rpcError, done, handleChanOut)
			}
		case req := <-c.requests:
			if req.req.ID != nil {
				c.inflight[*req.req.ID] = req
			}
			sendReq(req.req)
		case <-c.stop:
			if err := c.conn.Close(); err != nil {
				log.Debugf("websocket close error", "error", err)
			}
			return
		}
	}
}
