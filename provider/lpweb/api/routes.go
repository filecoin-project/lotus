// Package api provides the HTTP API for the lotus provider web gui.
package api

import (
	"github.com/gorilla/mux"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/provider/lpweb/api/debug"
)

func Routes(r *mux.Router, deps *deps.Deps) {
	debug.Routes(r.PathPrefix("/debug").Subrouter(), deps)
}
