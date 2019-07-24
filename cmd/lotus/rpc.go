package main

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func serveRPC(a api.FullNode, addr string) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", api.PermissionedFullAPI(a))
	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(addr, http.DefaultServeMux)
}
