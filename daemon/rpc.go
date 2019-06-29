package daemon

import (
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/rpclib"
)


func serveRPC(api api.API) error {
	rpcServer := rpclib.NewServer()
	rpcServer.Register("Filecoin", api)
	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(":1234", http.DefaultServeMux)
}
