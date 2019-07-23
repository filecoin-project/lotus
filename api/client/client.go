package client

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

// NewRPC creates a new http jsonrpc client.
func NewRPC(addr string, requestHeader http.Header) (api.API, error) {
	var res api.Struct
	_, err := jsonrpc.NewClient(addr, "Filecoin", &res.Internal, requestHeader)
	return &res, err
}
