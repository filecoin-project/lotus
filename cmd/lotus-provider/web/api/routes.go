// Package api provides the HTTP API for the lotus provider web gui.
package api

import (
	"github.com/filecoin-project/lotus/cmd/lotus-provider/web/api/debug"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/gorilla/mux"
)

func Routes(r *mux.Router, db *harmonydb.DB) {
	debug.Routes(r, db)
}
