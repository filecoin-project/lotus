package client

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/rpclib"
)

// NewRPC creates a new http jsonrpc client.
func NewRPC(addr string) api.API {
	var res api.Struct
	rpclib.NewClient(addr, "Filecoin", &res.Internal)
	return &res
}
