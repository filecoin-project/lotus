package blockstore

import (
	"bytes"
	"context"

	"github.com/gorilla/websocket"
	"github.com/libp2p/go-msgio"
	"golang.org/x/xerrors"
)

type wsWrapper struct {
	wc *websocket.Conn

	nextMsg []byte
}

func (w *wsWrapper) Read(b []byte) (int, error) {
	return 0, xerrors.New("read unsupported")
}

func (w *wsWrapper) ReadMsg() ([]byte, error) {
	if w.nextMsg != nil {
		nm := w.nextMsg
		w.nextMsg = nil
		return nm, nil
	}

	mt, r, err := w.wc.NextReader()
	if err != nil {
		return nil, err
	}

	switch mt {
	case websocket.BinaryMessage, websocket.TextMessage:
	default:
		return nil, xerrors.Errorf("unexpected message type")
	}

	// todo pool
	// todo limit sizes
	var mbuf bytes.Buffer
	if _, err := mbuf.ReadFrom(r); err != nil {
		return nil, err
	}

	return mbuf.Bytes(), nil
}

func (w *wsWrapper) ReleaseMsg(bytes []byte) {
	// todo use a pool
}

func (w *wsWrapper) NextMsgLen() (int, error) {
	if w.nextMsg != nil {
		return len(w.nextMsg), nil
	}

	mt, msg, err := w.wc.ReadMessage()
	if err != nil {
		return 0, err
	}

	switch mt {
	case websocket.BinaryMessage, websocket.TextMessage:
	default:
		return 0, xerrors.Errorf("unexpected message type")
	}

	w.nextMsg = msg
	return len(w.nextMsg), nil
}

func (w *wsWrapper) Write(bytes []byte) (int, error) {
	return 0, xerrors.New("write unsupported")
}

func (w *wsWrapper) WriteMsg(bytes []byte) error {
	return w.wc.WriteMessage(websocket.BinaryMessage, bytes)
}

func (w *wsWrapper) Close() error {
	return w.wc.Close()
}

var _ msgio.ReadWriteCloser = &wsWrapper{}

func wsConnToMio(wc *websocket.Conn) msgio.ReadWriteCloser {
	return &wsWrapper{
		wc: wc,
	}
}

func HandleNetBstoreWS(ctx context.Context, bs Blockstore, wc *websocket.Conn) *NetworkStoreHandler {
	return HandleNetBstoreStream(ctx, bs, wsConnToMio(wc))
}

func NewNetworkStoreWS(wc *websocket.Conn) *NetworkStore {
	return NewNetworkStore(wsConnToMio(wc))
}
