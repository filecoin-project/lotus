package remotewallet

import (
	"context"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
	"net/http"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type RemoteWallet struct {
	api.WalletAPI
}

func SetupRemoteWallet(url string) func(mctx helpers.MetricsCtx, lc fx.Lifecycle) (*RemoteWallet, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle) (*RemoteWallet, error) {
		/*sp := strings.SplitN(env, ":", 2)
		if len(sp) != 2 {
			log.Warnf("invalid env(%s) value, missing token or address", envKey)
		} else {
			ma, err := multiaddr.NewMultiaddr(sp[1])
			if err != nil {
				return APIInfo{}, xerrors.Errorf("could not parse multiaddr from env(%s): %w", envKey, err)
			}
			return APIInfo{
				Addr:  ma,
				Token: []byte(sp[0]),
			}, nil
		}*/

		headers := http.Header{}
		/*headers.Add("Authorization", "Bearer "+token)*/

		wapi, closer, err := client.NewWalletRPC(mctx, url, headers)
		if err != nil {
			return nil, xerrors.Errorf("creating jsonrpc client: %w", err)
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				closer()
				return nil
			},
		})

		return &RemoteWallet{wapi}, nil
	}
}