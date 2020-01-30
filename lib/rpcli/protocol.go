package rpcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"golang.org/x/xerrors"
)

var (
	errorType   = reflect.TypeOf(new(error)).Elem()
	contextType = reflect.TypeOf(new(context.Context)).Elem()
)

const (
	wsCancel = "xrpc.cancel"
	chValue  = "xrpc.ch.val"
	chClose  = "xrpc.ch.close"
)

// common errors
var (
	ErrContextDone = xerrors.New("context done")

	ErrNoResponse       = xerrors.New("no response received")
	ErrEmptyDataFrame   = xerrors.New("received an empty data frame")
	ErrNilDataFrame     = xerrors.New("received a nil data frame")
	ErrJSONUnmarshaling = xerrors.New("json unmarshaling failed")

	ErrUnexpectedMessageType = xerrors.New("unexpected message type")
	ErrMalformedFrame        = xerrors.New("malformed frame data")

	ErrNoAvailableConnection = xerrors.New("no available websocket connection")

	ErrClientClosed    = xerrors.New("rpc client closed")
	ErrRequestFinished = xerrors.New("rpc request finished")

	ErrBadHandshake = xerrors.New("bad handshake")
)

type param struct {
	data []byte // from unmarshal

	v reflect.Value // to marshal
}

func (p *param) UnmarshalJSON(raw []byte) error {
	p.data = make([]byte, len(raw))
	copy(p.data, raw)
	return nil
}

func (p *param) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.v.Interface())
}

type respError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *respError) Error() string {
	if e.Code >= -32768 && e.Code <= -32000 {
		return fmt.Sprintf("RPC error (%d): %s", e.Code, e.Message)
	}
	return e.Message
}

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

type dataOrErr struct {
	connID *uint64
	err    error

	// call r.Close() to release pooled buffer
	// maybe use a interface named ReadReleaser here?
	r io.ReadCloser
}

func (de *dataOrErr) extractJSON(v interface{}, required bool) error {
	if de == nil {
		return ErrNilDataFrame
	}

	if de.err != nil {
		return de.err
	}

	if de.r == nil {
		if required {
			return ErrEmptyDataFrame
		}

		return nil
	}

	defer de.r.Close()

	if err := json.NewDecoder(de.r).Decode(v); err != nil {
		return xerrors.Errorf("%w: %s", ErrJSONUnmarshaling, err)
	}

	return nil
}
