package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/gorilla/websocket"
)

type frame struct {
	// common
	Jsonrpc string     `json:"jsonrpc"`
	ID      *int64      `json:"id,omitempty"`

	// request
	Method  string  `json:"method,omitempty"`
	Params  []param `json:"params,omitempty"`

	// response
	Result  result     `json:"result,omitempty"`
	Error   *respError `json:"error,omitempty"`
}

func handleWsConn(ctx context.Context, conn *websocket.Conn, handler handlers, requests <-chan clientRequest, stop <-chan struct{}) {
	incoming := make(chan io.Reader)

	nextMessage := func() {
		mtype, r, err := conn.NextReader()
		if err != nil {
			r, _ := io.Pipe()
			r.CloseWithError(err) // nolint
			incoming <- r
			return

		}
		if mtype != websocket.BinaryMessage && mtype != websocket.TextMessage {
			r, _ := io.Pipe()
			r.CloseWithError(errors.New("unsupported message type")) // nolint
			incoming <- r
			return
		}
		incoming <- r
	}

	go nextMessage()

	inflight := map[int64]clientRequest{}

	for {
		select {
		case r := <-incoming:
			var frame frame
			if err := json.NewDecoder(r).Decode(&frame); err != nil {
				log.Error("handle me:", err)
				return
			}

			if frame.Method != "" {
				// call
				req := request{
					Jsonrpc: frame.Jsonrpc,
					ID:      frame.ID,
					Method:  frame.Method,
					Params:  frame.Params,
				}

				// TODO: ignore ID
				wcl, err := conn.NextWriter(websocket.TextMessage)
				if err != nil {
					log.Error("handle me:", err)
					return
				}

				handler.handle(ctx, req, wcl, func(w io.Writer, req *request, code int, err error) {
					log.Error("handle me:", err) // TODO: seriously
					return
				})

				if err := wcl.Close(); err != nil {
					log.Error("handle me:", err)
					return
				}
			} else {
				// response
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
			}

			go nextMessage()
		case req := <-requests:
			inflight[*req.req.ID] = req
			if err := conn.WriteJSON(req.req); err != nil {
				log.Error("handle me:", err)
				return
			}
		case <-stop:
			return
		}
	}
}
