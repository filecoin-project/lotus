package main

import (
	"context"
	"encoding/json"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"golang.org/x/xerrors"

	"contrib.go.opencensus.io/exporter/prometheus"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
)

var log = logging.Logger("main")

func serveRPC(a api.FullNode, stop node.StopFunc, addr multiaddr.Multiaddr) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", apistruct.PermissionedFullAPI(a))

	ah := &auth.Handler{
		Verify: a.AuthVerify,
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", ah)

	importAH := &auth.Handler{
		Verify: a.AuthVerify,
		Next:   handleImport(a.(*impl.FullNodeAPI)),
	}

	http.Handle("/rest/v0/import", importAH)

	exporter, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "lotus",
	})
	if err != nil {
		log.Fatalf("could not create the prometheus stats exporter: %v", err)
	}

	http.Handle("/debug/metrics", exporter)

	lst, err := manet.Listen(addr)
	if err != nil {
		return xerrors.Errorf("could not listen: %w", err)
	}

	srv := &http.Server{Handler: http.DefaultServeMux}

	sigChan := make(chan os.Signal, 2)
	go func() {
		<-sigChan
		if err := srv.Shutdown(context.TODO()); err != nil {
			log.Errorf("shutting down RPC server failed: %s", err)
		}
		if err := stop(context.TODO()); err != nil {
			log.Errorf("graceful shutting down failed: %s", err)
		}
	}()
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	return srv.Serve(manet.NetListener(lst))
}

func handleImport(a *impl.FullNodeAPI) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			w.WriteHeader(404)
			return
		}
		if !auth.HasPerm(r.Context(), nil, apistruct.PermWrite) {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
			return
		}

		c, err := a.ClientImportLocal(r.Context(), r.Body)
		if err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(struct{ Error string }{err.Error()})
			return
		}
		w.WriteHeader(200)
		err = json.NewEncoder(w).Encode(struct{ Cid cid.Cid }{c})
		if err != nil {
			log.Errorf("/rest/v0/import: Writing response failed: %+v", err)
			return
		}
	}
}
