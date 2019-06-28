package daemon

import (
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/rpclib"
)

type Filecoin struct{}

func (*Filecoin) ServerVersion(in int) (string, error) {
	fmt.Println(in)
	return build.Version, nil
}

func serveRPC() error {
	fc := new(Filecoin)

	rpcServer := rpclib.NewServer()

	rpcServer.Register(fc)

	http.Handle("/rpc/v0", rpcServer)
	return http.ListenAndServe(":1234", http.DefaultServeMux)
}
