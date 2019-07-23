package daemon

import (
	"context"
	"github.com/filecoin-project/go-lotus/lib/auth"
	"net/http"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func serveRPC(a api.API, addr string, verify func(ctx context.Context, token string) ([]string, error)) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", api.Permissioned(a))

	authHandler := &auth.Handler{
		Verify: verify,
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", authHandler)
	return http.ListenAndServe(addr, http.DefaultServeMux)
}
