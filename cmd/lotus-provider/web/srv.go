// Package web defines the HTTP web server for static files and endpoints.
package web

import (
	"context"
	"embed"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/web/api"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/web/hapi"
	"github.com/filecoin-project/lotus/metrics"
)

// go:embed static
var static embed.FS

func GetSrv(ctx context.Context, deps *deps.Deps) (*http.Server, error) {
	mux := mux.NewRouter()
	api.Routes(mux.PathPrefix("/api").Subrouter(), deps)
	err := hapi.Routes(mux.PathPrefix("/hapi").Subrouter(), deps)
	if err != nil {
		return nil, err
	}
	mux.NotFoundHandler = http.FileServer(http.FS(static))

	return &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/") {
				r.URL.Path = r.URL.Path + "index.html"
				return
			}
			mux.ServeHTTP(w, r)
		}),
		BaseContext: func(listener net.Listener) context.Context {
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-provider"))
			return ctx
		},
		Addr: deps.Cfg.Subsystems.GuiAddress,
	}, nil
}
