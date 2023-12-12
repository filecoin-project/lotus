// Package debug provides the API for various debug endpoints in lotus-provider.
package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/lotus/api/client"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"golang.org/x/xerrors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/build"
)

var log = logging.Logger("lp/web/debug")

type debug struct {
	db *harmonydb.DB
}

func Routes(r *mux.Router, db *harmonydb.DB) {
	d := debug{db}
	r.Methods("GET").Path("/api/debug/chain-state-sse").HandlerFunc(d.chainStateSSE)
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

	ctx := r.Context()

	for {

		type minimalApiInfo struct {
			Apis struct {
				ChainApiInfo []string
			}
		}

		rpcInfos := map[string]minimalApiInfo{} // config name -> api info
		confNameToAddr := map[string]string{}   // config name -> api address

		err := forEachConfig[minimalApiInfo](d, func(name string, info minimalApiInfo) error {
			if len(info.Apis.ChainApiInfo) == 0 {
				return nil
			}

			rpcInfos[name] = info

			for _, addr := range info.Apis.ChainApiInfo {
				ai := cliutil.ParseApiInfo(addr)
				confNameToAddr[name] = ai.Addr
			}

			return nil
		})
		if err != nil {
			log.Errorw("getting api info", "error", err)
			return
		}

		apiInfos := map[string][]byte{} // api address -> token
		// for dedup by address
		for _, info := range rpcInfos {
			ai := cliutil.ParseApiInfo(info.Apis.ChainApiInfo[0])
			apiInfos[ai.Addr] = ai.Token
		}

		infos := map[string]rpcInfo{} // api address -> rpc info
		var infosLk sync.Mutex

		var wg sync.WaitGroup
		wg.Add(len(rpcInfos))
		for addr, token := range apiInfos {
			ai := cliutil.APIInfo{
				Addr:  addr,
				Token: token,
			}

			var clayers []string
			for layer, a := range confNameToAddr {
				if a == addr {
					clayers = append(clayers, layer)
				}
			}

			da, err := ai.DialArgs("v1")
			if err != nil {
				log.Warnw("DialArgs", "error", err)

				infosLk.Lock()
				infos[addr] = rpcInfo{
					Address:   ai.Addr,
					Reachable: false,
					CLayers:   clayers,
				}
				infosLk.Unlock()

				wg.Done()
				continue
			}

			ah := ai.AuthHeader()

			v1api, closer, err := client.NewFullNodeRPCV1(ctx, da, ah)
			if err != nil {
				log.Warnf("Not able to establish connection to node with addr: %s", addr)

				infosLk.Lock()
				infos[addr] = rpcInfo{
					Address:   ai.Addr,
					Reachable: false,
					CLayers:   clayers,
				}
				infosLk.Unlock()

				wg.Done()
				continue
			}

			go func(info string) {
				defer wg.Done()
				defer closer()

				ver, err := v1api.Version(ctx)
				if err != nil {
					log.Warnw("Version", "error", err)
					return
				}

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
					syncState = fmt.Sprintf("slow (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second))
				default:
					syncState = fmt.Sprintf("behind (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second))
				}

				var out rpcInfo
				out.Address = ai.Addr
				out.CLayers = clayers
				out.Reachable = true
				out.Version = ver.Version
				out.SyncState = syncState

				infosLk.Lock()
				infos[info] = out
				infosLk.Unlock()

			}(addr)
		}
		wg.Wait()

		infoList := make([]rpcInfo, 0, len(infos))
		for _, info := range infos {
			infoList = append(infoList, info)
		}

		sort.Slice(infoList, func(i, j int) bool {
			return infoList[i].Address < infoList[j].Address
		})

		fmt.Fprintf(w, "data: ")
		err = json.NewEncoder(w).Encode(&infoList)
		if err != nil {
			log.Warnw("json encode", "error", err)
			return
		}
		fmt.Fprintf(w, "\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		time.Sleep(time.Duration(build.BlockDelaySecs) * time.Second)
	}
}

func forEachConfig[T any](a *debug, cb func(name string, v T) error) error {
	confs, err := a.loadConfigs(context.Background())
	if err != nil {
		return err
	}

	for name, tomlStr := range confs { // todo for-each-config
		var info T
		if err := toml.Unmarshal([]byte(tomlStr), &info); err != nil {
			return xerrors.Errorf("unmarshaling %s config: %w", name, err)
		}

		if err := cb(name, info); err != nil {
			return xerrors.Errorf("cb: %w", err)
		}
	}

	return nil
}

func (d *debug) loadConfigs(ctx context.Context) (map[string]string, error) {
	//err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)

	rows, err := d.db.Query(ctx, `SELECT title, config FROM harmony_config`)
	if err != nil {
		return nil, xerrors.Errorf("getting db configs: %w", err)
	}

	configs := make(map[string]string)
	for rows.Next() {
		var title, config string
		if err := rows.Scan(&title, &config); err != nil {
			return nil, xerrors.Errorf("scanning db configs: %w", err)
		}
		configs[title] = config
	}

	return configs, nil
}
