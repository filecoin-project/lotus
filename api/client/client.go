package client

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib"
)

// NewRPC creates a new http jsonrpc client.
func NewRPC(addr string) api.API {
	var res api.Struct
	lib.NewClient(addr, "Filecoin", &res.Internal)
	return &res
}
