package daemon

import (
	"net/http"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json"

	"github.com/filecoin-project/go-lotus/build"
)

type Filecoin struct {}

func (*Filecoin) ServerVersion(r *http.Request, _ *struct{}, out *string) error {
	*out = build.Version
	return nil
}

func serveRPC() error {
	fc := new(Filecoin)

	rpcServer := rpc.NewServer()
	rpcServer.RegisterCodec(json.NewCodec(), "application/json")
	if err := rpcServer.RegisterService(fc, ""); err != nil {
		return err
	}

	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(":1234", http.DefaultServeMux)
}

