package modules

import (
	"context"
	"crypto/rand"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/lib/addrutil"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"time"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/node/repo"
)

var log = logging.Logger("modules")

type Genesis func() (*types.BlockHeader, error)

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk": record.PublicKeyValidator{},
	}
}

const JWTSecretName = "auth-jwt-private"

type jwtPayload struct {
	Allow []string
}

func APISecret(keystore types.KeyStore, lr repo.LockedRepo) (*dtypes.APIAlg, error) {
	key, err := keystore.Get(JWTSecretName)
	if err != nil {
		log.Warn("Generating new API secret")

		sk, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 32))
		if err != nil {
			return nil, err
		}

		key = types.KeyInfo{
			Type:       "jwt-hmac-secret",
			PrivateKey: sk,
		}

		if err := keystore.Put(JWTSecretName, key); err != nil {
			return nil, xerrors.Errorf("writing API secret: %w", err)
		}

		// TODO: make this configurable
		p := jwtPayload{
			Allow: api.AllPermissions,
		}

		cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.PrivateKey))
		if err != nil {
			return nil, err
		}

		if err := lr.SetAPIToken(cliToken); err != nil {
			return nil, err
		}
	}

	return (*dtypes.APIAlg)(jwt.NewHS256(key.PrivateKey)), nil
}

func ConfigBootstrap(peers []string) func() (dtypes.BootstrapPeers, error) {
	return func() (dtypes.BootstrapPeers, error) {
		return addrutil.ParseAddresses(context.TODO(), peers)
	}
}

func BuiltinBootstrap() (dtypes.BootstrapPeers, error) {
	return build.BuiltinBootstrap()
}

func Bootstrap(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, pinfos dtypes.BootstrapPeers) {
	ctx, cancel := context.WithCancel(mctx)

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				for {
					sctx, cancel := context.WithTimeout(ctx, build.BlockDelay*time.Second/2)
					<-sctx.Done()
					cancel()

					if ctx.Err() != nil {
						return
					}

					if len(host.Network().Conns()) > 0 {
						continue
					}

					log.Warn("No peers connected, performing automatic bootstrap")

					for _, pi := range pinfos {
						if err := host.Connect(ctx, pi); err != nil {
							log.Warn("bootstrap connect failed: ", err)
						}
					}
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})
}
