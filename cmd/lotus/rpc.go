package main

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/auth"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func serveRPC(a api.FullNode, addr string) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", api.PermissionedFullAPI(a))

	ah := &auth.Handler{
		Verify: a.AuthVerify,
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", ah)
	return http.ListenAndServe(addr, http.DefaultServeMux)
}
