package rpcli

import (
	"context"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

// Connector dials with inside context and returns established connection or any failure
type Connector interface {
	Dial(context.Context) (RawConn, error)
}

// NewWebsocketConnector returns a connector with given infomation
func NewWebsocketConnector(endpoint string, header http.Header) Connector {
	return &wsConnector{
		endpoint: endpoint,
		header:   header,
		idgen:    &simpleIDGen{},
	}
}

type wsConnector struct {
	endpoint string
	header   http.Header
	idgen    idGenerator
}

func (wsc *wsConnector) Dial(ctx context.Context) (RawConn, error) {
	wsconn, _, err := websocket.DefaultDialer.DialContext(ctx, wsc.endpoint, wsc.header)
	if err != nil {
		return nil, err
	}

	return &wsConn{
		id: wsc.idgen.next(),
		ws: wsconn,
	}, nil
}

// RawConn defines a bio-direction connection, and can be mocked for testing
type RawConn interface {
	startIncomingLoop(ctx context.Context) <-chan rawMsgOrErr
	sendFrame(ctx context.Context, f frame) error
}

var _ RawConn = (*wsConn)(nil)

// wsConn is a simple wrapper around websocket.wsConn
type wsConn struct {
	id uint64
	ws *websocket.Conn
}

func (c *wsConn) startIncomingLoop(ctx context.Context) <-chan rawMsgOrErr {
	out := make(chan rawMsgOrErr, 1)
	go func() {

		defer func() {
			close(out)
			c.ws.Close()
		}()

		onNext := func(r io.Reader) {
			select {
			case <-ctx.Done():

			case out <- rawMsgOrErr{
				connID: c.id,
				r:      r,
			}:

			}
		}

		onErr := func(err error) {
			select {
			case <-ctx.Done():

			case out <- rawMsgOrErr{
				connID: c.id,
				err:    err,
			}:

			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			default:

			}

			mtype, reader, err := c.ws.NextReader()
			if err != nil {
				onErr(err)
				return
			}

			if mtype != websocket.BinaryMessage && mtype != websocket.TextMessage {
				onErr(ErrUnexpectedMessageType)
				return
			}

			onNext(reader)
		}
	}()

	return out
}

func (c *wsConn) sendFrame(ctx context.Context, f frame) error {
	if c == nil {
		return ErrNoAvailableConnection
	}

	err := c.ws.WriteJSON(f)
	if err == nil {
		return nil
	}

	if isWsConnectionErr(err) {
		return ErrNoAvailableConnection
	}

	return err
}

func isWsConnectionErr(err error) bool {
	if err == nil {
		return false
	}

	if err == ErrNoAvailableConnection {
		return true
	}

	// see RFC 6455
	// https://tools.ietf.org/html/rfc6455#section-7.4.1
	return websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseProtocolError,
		websocket.CloseUnsupportedData,
		websocket.CloseInvalidFramePayloadData,
		websocket.ClosePolicyViolation,
		// shall we handle `message too big` in a special way?
		websocket.CloseMessageTooBig,
	)
}

type rawMsgOrErr struct {
	connID uint64
	r      io.Reader
	err    error
}
