package daemon

import (
	"github.com/filecoin-project/go-lotus/lib"
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
)

func serveRPC(api api.API) error {
	rpcServer := lib.NewServer()
	rpcServer.Register("Filecoin", api)
	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(":1234", http.DefaultServeMux)
}
