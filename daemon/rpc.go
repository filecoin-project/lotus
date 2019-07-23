package daemon

import (
	"github.com/filecoin-project/go-lotus/lib/auth"
	"github.com/gbrlsnchs/jwt/v3"
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func serveRPC(a api.API, addr string, authSecret []byte) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", api.Permissioned(a))

	authHandler := &auth.Handler{
		Secret: jwt.NewHS256(authSecret),
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", authHandler)
	return http.ListenAndServe(addr, http.DefaultServeMux)
}
