package jsonrpc

import (
	"encoding/json"
	"io"
	"net/http"

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

var upgrader = websocket.Upgrader{}

func (s *RPCServer) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	(&wsConn{
		conn:     c,
		handler:  s.methods,
	}).handleWsConn(r.Context())

	if err := c.Close(); err != nil {
		log.Error(err)
		return
	}
}

// TODO: return errors to clients per spec
func (s *RPCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Connection") == "Upgrade" {
		s.handleWS(w, r)
		return
	}

	s.methods.handleReader(r.Context(), r.Body, w, rpcError)
}

func rpcError(wf func(func(io.Writer)), req *request, code int, err error) {
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
