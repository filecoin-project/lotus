package lpweb

import (
	"context"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/build"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"golang.org/x/xerrors"
	"sync"
	"time"
)

func (a *app) loadConfigs(ctx context.Context) (map[string]string, error) {
	//err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)

	rows, err := a.db.Query(ctx, `SELECT title, config FROM harmony_config`)
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

var watchInterval = 5 * time.Second

func (a *app) watchRpc() {
	ticker := time.NewTicker(watchInterval)
	for {
		err := a.updateRpc(context.TODO())
		if err != nil {
			log.Errorw("updating rpc info", "error", err)
		}
		select {
		case <-ticker.C:
		}
	}
}

type minimalApiInfo struct {
	Apis struct {
		ChainApiInfo []string
	}
}

func (a *app) updateRpc(ctx context.Context) error {
	confs, err := a.loadConfigs(context.Background())
	if err != nil {
		return err
	}

	rpcInfos := map[string]minimalApiInfo{} // config name -> api info
	confNameToAddr := map[string]string{}   // config name -> api address

	for name, tomlStr := range confs {
		var info minimalApiInfo
		if err := toml.Unmarshal([]byte(tomlStr), &info); err != nil {
			return xerrors.Errorf("unmarshaling %s config: %w", name, err)
		}

		if len(info.Apis.ChainApiInfo) == 0 {
			continue
		}

		rpcInfos[name] = info

		for _, addr := range info.Apis.ChainApiInfo {
			ai := cliutil.ParseApiInfo(addr)
			confNameToAddr[name] = ai.Addr
		}
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

		da, err := ai.DialArgs("v1")
		if err != nil {
			log.Warnw("DialArgs", "error", err)

			infosLk.Lock()
			infos[addr] = rpcInfo{
				Address:   ai.Addr,
				Reachable: false,
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

			var clayers []string
			for layer, addr := range confNameToAddr {
				if addr == info {
					clayers = append(clayers, layer)
				}
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

	a.rpcInfoLk.Lock()
	a.rpcInfos = make([]rpcInfo, 0, len(infos))
	for _, info := range infos {
		a.rpcInfos = append(a.rpcInfos, info)
	}
	a.rpcInfoLk.Unlock()

	return nil
}
