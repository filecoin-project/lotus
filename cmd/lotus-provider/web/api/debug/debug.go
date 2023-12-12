// Package debug provides the API for various debug endpoints in lotus-provider.
package debug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/build"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
)

var log = logging.Logger("lp/web/debug")

type debug struct {
	*deps.Deps
}

func Routes(r *mux.Router, deps *deps.Deps) {
	d := debug{deps}
	r.Methods("GET").Path("chain-state-sse").HandlerFunc(d.chainStateSSE)
}

type rpcInfo struct {
	Address   string
	CLayers   []string
	Reachable bool
	SyncState string
	Version   string
}

func (d *debug) chainStateSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	v1api := d.Deps.Full
	ctx := r.Context()

	ai := cliutil.ParseApiInfo(d.Deps.Cfg.Apis.ChainApiInfo[0])
	ver, err := v1api.Version(ctx)
	if err != nil {
		log.Warnw("Version", "error", err)
		return
	}

sse:
	for {
		head, err := v1api.ChainHead(ctx)
		if err != nil {
			log.Warnw("ChainHead", "error", err)
			return
		}

		var syncState string
		switch {
		case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*3/2): // within 1.5 epochs
			syncState = "ok"
		case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*5): // within 5 epochs
			syncState = fmt.Sprintf("slow (%s behind)", time.Since(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second))
		default:
			syncState = fmt.Sprintf("behind (%s behind)", time.Since(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second))
		}

		select {
		case <-ctx.Done():
			break sse
		default:
		}

		fmt.Fprintf(w, "data: ")
		err = json.NewEncoder(w).Encode(rpcInfo{
			Address:   ai.Addr,
			CLayers:   []string{},
			Reachable: true,
			Version:   ver.Version,
			SyncState: syncState,
		})
		if err != nil {
			log.Warnw("json encode", "error", err)
			return
		}
		fmt.Fprintf(w, "\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
