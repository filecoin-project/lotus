package modules

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/ioutil"

	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/addrutil"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("modules")

type Genesis func() (*types.BlockHeader, error)

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk": record.PublicKeyValidator{},
	}
}

const JWTSecretName = "auth-jwt-private"  //nolint:gosec
const KTJwtHmacSecret = "jwt-hmac-secret" //nolint:gosec

type JwtPayload struct {
	Allow []auth.Permission
}

func APISecret(keystore types.KeyStore, lr repo.LockedRepo) (*dtypes.APIAlg, error) {
	key, err := keystore.Get(JWTSecretName)

	if errors.Is(err, types.ErrKeyInfoNotFound) {
		log.Warn("Generating new API secret")

		sk, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 32))
		if err != nil {
			return nil, err
		}

		key = types.KeyInfo{
			Type:       KTJwtHmacSecret,
			PrivateKey: sk,
		}

		if err := keystore.Put(JWTSecretName, key); err != nil {
			return nil, xerrors.Errorf("writing API secret: %w", err)
		}

		// TODO: make this configurable
		p := JwtPayload{
			Allow: apistruct.AllPermissions,
		}

		cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.PrivateKey))
		if err != nil {
			return nil, err
		}

		if err := lr.SetAPIToken(cliToken); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, xerrors.Errorf("could not get JWT Token: %w", err)
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

func DrandBootstrap(ds dtypes.DrandSchedule) (dtypes.DrandBootstrap, error) {
	// TODO: retry resolving, don't fail if at least one resolve succeeds
	res := []peer.AddrInfo{}
	for _, d := range ds {
		addrs, err := addrutil.ParseAddresses(context.TODO(), d.Config.Relays)
		if err != nil {
			log.Errorf("reoslving drand relays addresses: %+v", err)
			continue
		}
		res = append(res, addrs...)
	}
	return res, nil
}

func NewDefaultMaxFeeFunc(r repo.LockedRepo) dtypes.DefaultMaxFeeFunc {
	return func() (out abi.TokenAmount, err error) {
		err = readNodeCfg(r, func(cfg *config.FullNode) {
			out = abi.TokenAmount(cfg.Fees.DefaultMaxFee)
		})
		return
	}
}

func readNodeCfg(r repo.LockedRepo, accessor func(node *config.FullNode)) error {
	raw, err := r.Config()
	if err != nil {
		return err
	}

	cfg, ok := raw.(*config.FullNode)
	if !ok {
		return xerrors.New("expected config.FullNode")
	}

	accessor(cfg)

	return nil
}
