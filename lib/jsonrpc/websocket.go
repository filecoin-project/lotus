package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"sync"

	"github.com/gorilla/websocket"
)

const wsCancel = "xrpc.cancel"

type frame struct {
	// common
	Jsonrpc string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`

	// request
	Method string  `json:"method,omitempty"`
	Params []param `json:"params,omitempty"`

	// response
	Result result     `json:"result,omitempty"`
	Error  *respError `json:"error,omitempty"`
}

func handleWsConn(ctx context.Context, conn *websocket.Conn, handler handlers, requests <-chan clientRequest, stop <-chan struct{}) {
	incoming := make(chan io.Reader)
	var incErr error

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

	go nextMessage()

	inflight := map[int64]clientRequest{}
	handling := map[int64]context.CancelFunc{}
	var handlingLk sync.Mutex

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

			var frame frame
			if err := json.NewDecoder(r).Decode(&frame); err != nil {
				log.Error("handle me:", err)
				return
			}

			switch frame.Method {
			case "": // Response to our call
				req, ok := inflight[*frame.ID]
				if !ok {
					log.Error("client got unknown ID in response")
					continue
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
				done := cf
				if frame.ID != nil {
					nw = nextWriter

					handlingLk.Lock()
					handling[*frame.ID] = cf
					handlingLk.Unlock()

					done = func() {
						handlingLk.Lock()
						defer handlingLk.Unlock()

						cf()
						delete(handling, *frame.ID)
					}
				}

				go handler.handle(ctx, req, nw, rpcError, done)
			}

			go nextMessage()
		case req := <-requests:
			if req.req.ID != nil {
				inflight[*req.req.ID] = req
			}
			if err := conn.WriteJSON(req.req); err != nil {
				log.Error("handle me:", err)
				return
			}
		case <-stop:
			if err := conn.Close(); err != nil {
				log.Debugf("websocket close error", "error", err)
			}
			return
		}
	}
}
