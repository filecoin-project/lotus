package daemon

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func serveRPC(a api.API, addr string) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", api.Permissioned(a))
	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(addr, http.DefaultServeMux)
}
