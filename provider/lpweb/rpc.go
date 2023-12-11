package lpweb

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

type webApp struct {
	db   *harmonydb.DB
	full api.FullNode
}

func ServeWeb(listen string) error {
	return nil
}
