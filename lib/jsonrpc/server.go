package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

const (
	rpcParseError     = -32700
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

// RPCServer provides a jsonrpc 2.0 http server handler
type RPCServer struct {
	methods handlers
}

// NewServer creates new RPCServer instance
func NewServer() *RPCServer {
	return &RPCServer{
		methods: map[string]rpcHandler{},
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *RPCServer) handleWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// TODO: allow setting
	// (note that we still are mostly covered by jwt tokens)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Header.Get("Sec-WebSocket-Protocol") != "" {
		w.Header().Set("Sec-WebSocket-Protocol", r.Header.Get("Sec-WebSocket-Protocol"))
	}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	(&wsConn{
		conn:    c,
		handler: s.methods,
		exiting: make(chan struct{}),
	}).handleWsConn(ctx)

	if err := c.Close(); err != nil {
		log.Error(err)
		return
	}
}

// TODO: return errors to clients per spec
func (s *RPCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h := strings.ToLower(r.Header.Get("Connection"))
	if strings.Contains(h, "upgrade") {
		s.handleWS(ctx, w, r)
		return
	}

	s.methods.handleReader(ctx, r.Body, w, rpcError)
}

func rpcError(wf func(func(io.Writer)), req *request, code int, err error) {
	log.Errorf("RPC Error: %s", err)
	wf(func(w io.Writer) {
		if hw, ok := w.(http.ResponseWriter); ok {
			hw.WriteHeader(500)
		}

		log.Warnf("rpc error: %s", err)

		if req.ID == nil { // notification
			return
		}

		resp := response{
			Jsonrpc: "2.0",
			ID:      *req.ID,
			Error: &respError{
				Code:    code,
				Message: err.Error(),
			},
		}

		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			log.Warnf("failed to write rpc error: %s", err)
			return
		}
	})
}

// Register registers new RPC handler
//
// Handler is any value with methods defined
func (s *RPCServer) Register(namespace string, handler interface{}) {
	s.methods.register(namespace, handler)
}

var _ error = &respError{}
