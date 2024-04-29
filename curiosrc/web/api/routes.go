// Package api provides the HTTP API for the lotus curio web gui.
package api

import (
	"github.com/gorilla/mux"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web/api/config"
	"github.com/filecoin-project/lotus/curiosrc/web/api/debug"
	"github.com/filecoin-project/lotus/curiosrc/web/api/sector"
)

func Routes(r *mux.Router, deps *deps.Deps) {
	debug.Routes(r.PathPrefix("/debug").Subrouter(), deps)
	config.Routes(r.PathPrefix("/config").Subrouter(), deps)
	sector.Routes(r.PathPrefix("/sector").Subrouter(), deps)
}
