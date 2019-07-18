package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/gorilla/websocket"

	"github.com/filecoin-project/go-lotus/api"
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

func (s *RPCServer) handleWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	(&wsConn{
		conn:    c,
		handler: s.methods,
	}).handleWsConn(ctx)

	if err := c.Close(); err != nil {
		log.Error(err)
		return
	}
}

// TODO: return errors to clients per spec
func (s *RPCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token := r.Header.Get("Authorization")
	if token != "" {
		if !strings.HasPrefix(token, "Bearer ") {
			w.WriteHeader(401)
			return
		}
		token = token[len("Bearer "):]

		var payload jwtPayload
		if _, err := jwt.Verify([]byte(token), secret, &payload); err != nil {
			w.WriteHeader(401)
			return
		}

		ctx = api.WithPerm(ctx, payload.Allow)
	}

	if r.Header.Get("Connection") == "Upgrade" {
		s.handleWS(ctx, w, r)
		return
	}

	s.methods.handleReader(ctx, r.Body, w, rpcError)
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
