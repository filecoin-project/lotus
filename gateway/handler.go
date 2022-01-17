package gateway

import (
	"context"
	"crypto/rand"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/crypto"
	"net/http"
	"strings"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/gorilla/mux"
	promclient "github.com/prometheus/client_golang/prometheus"
)

var log = logging.Logger("gateway")

const KGatewayHost = "gateway-host"

// Handler returns a gateway http.Handler, to be mounted as-is on the server.
func Handler(a api.Gateway, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(opts...)
		rpcServer.Register("Filecoin", hnd)

		handler := &AuthHandler{Verify: a.AuthVerify, Next: rpcServer.ServeHTTP}

		m.Handle(path, handler)
	}

	ma := proxy.MetricedGatewayAPI(a)

	serveRpc("/rpc/v1", ma)
	serveRpc("/rpc/v0", api.Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), ma))

	registry := promclient.DefaultRegisterer.(*promclient.Registry)
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "lotus_gw",
	})
	if err != nil {
		return nil, err
	}
	m.Handle("/debug/metrics", exporter)
	m.PathPrefix("/").Handler(http.DefaultServeMux)

	/*ah := &auth.Handler{
		Verify: nodeApi.AuthVerify,
		Next:   mux.ServeHTTP,
	}*/

	return m, nil
}

func MakeGatewayKey(lr repo.LockedRepo) (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	kbytes, err := crypto.MarshalPrivateKey(pk)
	if err != nil {
		return nil, err
	}

	if err := ks.Put(KGatewayHost, types.KeyInfo{
		Type:       KGatewayHost,
		PrivateKey: kbytes,
	}); err != nil {
		return nil, err
	}

	return pk, nil
}

func GetGatewayKey(lr repo.LockedRepo) (types.KeyInfo, error) {
	ks, err := lr.KeyStore()
	if err != nil {
		return types.KeyInfo{}, err
	}

	ki, err := ks.Get(KGatewayHost)
	if err != nil {
		return types.KeyInfo{}, err
	}

	return ki, nil
}

type AuthHandler struct {
	Verify func(ctx context.Context, token string) (*api.GatewayPayload, error)
	Next   http.HandlerFunc
}

func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.FormValue("token")
		if token != "" {
			token = "Bearer " + token
		}
	}

	if token != "" {
		if !strings.HasPrefix(token, "Bearer ") {
			log.Warn("missing Bearer prefix in auth header")
			w.WriteHeader(401)
			return
		}
		token = strings.TrimPrefix(token, "Bearer ")

		payload, err := h.Verify(ctx, token)
		if err != nil {
			log.Warnf("JWT Verification failed (originating from %s): %s", r.RemoteAddr, err)
			w.WriteHeader(401)
			return
		}

		ctx = WithPerm(ctx, payload)
	}

	h.Next(w, r.WithContext(ctx))
}

func WithPerm(ctx context.Context, payload *api.GatewayPayload) context.Context {
	return context.WithValue(ctx, "payload", *payload)
}
