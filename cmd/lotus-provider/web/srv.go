// Package web defines the HTTP web server for static files and endpoints.
package web

import (
	"context"
	"embed"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/web/api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/gorilla/mux"
	"go.opencensus.io/tag"
	"net"
	"net/http"
)

//go:embed static
var static embed.FS

func GetSrv(ctx context.Context, db *harmonydb.DB, address string) (*http.Server, error) {
	m := mux.NewRouter()
	api.Routes(m, db)
	m.PathPrefix("/static").Handler(http.FileServer(http.FS(static))).Methods("GET")

	return &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/static/index.html", http.StatusFound)
				return
			}
			m.ServeHTTP(w, r)
		}),
		BaseContext: func(listener net.Listener) context.Context {
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-provider"))
			return ctx
		},
		Addr: address,
	}, nil
}
