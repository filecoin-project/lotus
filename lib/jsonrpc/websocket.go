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

func handleWsConn(ctx context.Context, conn *websocket.Conn, handler handlers, requests <-chan clientRequest, stop <-chan struct{}) {
	incoming := make(chan io.Reader)
	var incErr error

	// nextMessage wait for one message and puts it to the incoming channel
	nextMessage := func() {
		mtype, r, err := conn.NextReader()
		if err != nil {
			incErr = err
			close(incoming)
			return
		}
		if mtype != websocket.BinaryMessage && mtype != websocket.TextMessage {
			incErr = errors.New("unsupported message type")
			close(incoming)
			return
		}
		incoming <- r
	}

	var writeLk sync.Mutex

	// nextWriter waits for writeLk and invokes the cb callback with WS message
	// writer when the lock is acquired
	nextWriter := func(cb func(io.Writer)) {
		writeLk.Lock()
		defer writeLk.Unlock()

		wcl, err := conn.NextWriter(websocket.TextMessage)
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
		writeLk.Lock()
		if err := conn.WriteJSON(req); err != nil {
			log.Error("handle me:", err)
			writeLk.Unlock()
			return
		}
		writeLk.Unlock()
	}

	// wait for the first message
	go nextMessage()

	// inflight are requests we sent to the remote
	inflight := map[int64]clientRequest{}

	// handling are the calls we handle
	handling := map[int64]context.CancelFunc{}
	var handlingLk sync.Mutex

	// ////
	// Subscriptions (func() <-chan Typ - like methods)

	var chOnce sync.Once
	var outId uint64
	type chReg struct {
		id uint64
		ch reflect.Value
	}
	registerCh := make(chan chReg)
	defer close(registerCh)

	handleOutChans := func() {
		regV := reflect.ValueOf(registerCh)

		cases := []reflect.SelectCase{
			{ // registration chan always 0
				Dir:  reflect.SelectRecv,
				Chan: regV,
			},
		}
		var caseToId []uint64

		for {
			chosen, val, ok := reflect.Select(cases)

			if chosen == 0 { // control channel
				if !ok {
					// not closing any channels as we're on receiving end.
					// Also, context cancellation below should take care of any running
					// requests
					return
				}

				registration := val.Interface().(chReg)

				caseToId = append(caseToId, registration.id)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: registration.ch,
				})

				continue
			}

			if !ok {
				n := len(caseToId)
				if n > 0 {
					cases[chosen] = cases[n]
					caseToId[chosen-1] = caseToId[n-1]
				}

				cases = cases[:n]
				caseToId = caseToId[:n-1]
				continue
			}

			sendReq(request{
				Jsonrpc: "2.0",
				ID:      nil, // notification
				Method:  chValue,
				Params:  []param{{v: reflect.ValueOf(caseToId[chosen-1])}, {v: val}},
			})
		}
	}

	handleChanOut := func(ch reflect.Value) interface{} {
		chOnce.Do(func() {
			go handleOutChans()
		})
		id := atomic.AddUint64(&outId, 1)

		registerCh <- chReg{
			id: id,
			ch: ch,
		}

		return id
	}

	// client side subs

	chanHandlers := map[uint64]func(m []byte, ok bool){}

	// ////

	// on close, make sure to return from all pending calls, and cancel context
	//  on all calls we handle
	defer func() {
		for id, req := range inflight {
			req.ready <- clientResponse{
				Jsonrpc: "2.0",
				ID:      id,
				Error: &respError{
					Message: "handler: websocket connection closed",
				},
			}

			handlingLk.Lock()
			for _, cancel := range handling {
				cancel()
			}
			handlingLk.Unlock()
		}
	}()

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

		handlingLk.Lock()
		defer handlingLk.Unlock()

		cf, ok := handling[id]
		if ok {
			cf()
		}
	}

	for {
		select {
		case r, ok := <-incoming:
			if !ok {
				if incErr != nil {
					log.Debugf("websocket error", "error", incErr)
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

			// Get message type by method name:
			// "" - response
			// "xrpc.*" - builtin
			// anything else - incoming remote call
			switch frame.Method {
			case "": // Response to our call
				req, ok := inflight[*frame.ID]
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

					chanHandlers[chid] = req.retCh()
				}

				req.ready <- clientResponse{
					Jsonrpc: frame.Jsonrpc,
					Result:  frame.Result,
					ID:      *frame.ID,
					Error:   frame.Error,
				}
				delete(inflight, *frame.ID)
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

					handlingLk.Lock()
					handling[*frame.ID] = cf
					handlingLk.Unlock()

					done = func(keepctx bool) {
						handlingLk.Lock()
						defer handlingLk.Unlock()

						if !keepctx {
							cf()
						}

						delete(handling, *frame.ID)
					}
				}

				go handler.handle(ctx, req, nw, rpcError, done, handleChanOut)
			}

			go nextMessage() // TODO: fix on errors
		case req := <-requests:
			if req.req.ID != nil {
				inflight[*req.req.ID] = req
			}
			sendReq(req.req)
		case <-stop:
			if err := conn.Close(); err != nil {
				log.Debugf("websocket close error", "error", err)
			}
			return
		}
	}
}
