package hapi

import (
	"context"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	blocks "github.com/ipfs/go-block-format"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/store"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/must"
)

var ChainBlockCache = must.One(lru.New[blockstore.MhString, blocks.Block](4096))

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
	rpcInfos := map[string]minimalApiInfo{} // config name -> api info
	confNameToAddr := map[string]string{}   // config name -> api address

	err := forEachConfig[minimalApiInfo](a, func(name string, info minimalApiInfo) error {
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
		return err
	}

	apiInfos := map[string][]byte{} // api address -> token

	// for dedup by address
	for _, info := range rpcInfos {
		ai := cliutil.ParseApiInfo(info.Apis.ChainApiInfo[0])
		apiInfos[ai.Addr] = ai.Token
	}

	a.rpcInfoLk.Lock()

	// todo improve this shared rpc logic
	if a.workingApi == nil {
		for addr, token := range apiInfos {
			ai := cliutil.APIInfo{
				Addr:  addr,
				Token: token,
			}

			da, err := ai.DialArgs("v1")
			if err != nil {
				continue
			}

			ah := ai.AuthHeader()

			v1api, closer, err := client.NewFullNodeRPCV1(ctx, da, ah)
			if err != nil {
				continue
			}
			go func() {
				<-ctx.Done()
				closer()
			}()

			a.workingApi = v1api
			a.stor = store.ActorStore(ctx, blockstore.NewReadCachedBlockstore(blockstore.NewAPIBlockstore(a.workingApi), ChainBlockCache))
		}
	}

	a.rpcInfoLk.Unlock()

	return nil
}
