package apihelper

import (
	"net/http"
	"runtime/debug"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("cu/web/apihelper")

func OrHTTPFail(w http.ResponseWriter, err error) {
	if err != nil {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
		log.Errorw("http fail", "err", err, "stack", string(debug.Stack()))
		panic(err)
	}
}
