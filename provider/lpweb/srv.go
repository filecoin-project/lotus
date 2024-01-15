// Package lpweb defines the HTTP web server for static files and endpoints.
package lpweb

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/provider/lpweb/api"
	"github.com/filecoin-project/lotus/provider/lpweb/hapi"
)

//go:embed static
var static embed.FS

var basePath = "/static/"

// An dev mode hack for no-restart changes to static and templates.
// You still need to recomplie the binary for changes to go code.
var webDev = os.Getenv("LOTUS_WEB_DEV") == "1"

func GetSrv(ctx context.Context, deps *deps.Deps) (*http.Server, error) {
	mx := mux.NewRouter()
	err := hapi.Routes(mx.PathPrefix("/hapi").Subrouter(), deps)
	if err != nil {
		return nil, err
	}
	api.Routes(mx.PathPrefix("/api").Subrouter(), deps)

	var static fs.FS = static
	if webDev {
		static = os.DirFS("./provider/lpweb")
	}

	mx.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the request is for a directory, redirect to the index file.
		if strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path += "index.html"
		}

		file, err := static.Open(path.Join(basePath, r.URL.Path)[1:])
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 Not Found"))
			return
		}
		defer func() { _ = file.Close() }()

		fileInfo, err := file.Stat()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("500 Internal Server Error"))
			return
		}

		http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file.(io.ReadSeeker))
	})

	return &http.Server{
		Handler: http.HandlerFunc(mx.ServeHTTP),
		BaseContext: func(listener net.Listener) context.Context {
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-provider"))
			return ctx
		},
		Addr:              deps.Cfg.Subsystems.GuiAddress,
		ReadTimeout:       time.Minute * 3,
		ReadHeaderTimeout: time.Minute * 3, // lint
	}, nil
}
