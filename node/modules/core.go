package modules

import (
	"context"
	"crypto/rand"
	"github.com/filecoin-project/lotus/api/apistruct"
	"io"
	"io/ioutil"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/addrutil"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
	"golang.org/x/xerrors"
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
			Allow: apistruct.AllPermissions,
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
