package modules

import (
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/types"
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

type APIAlg jwt.HMACSHA

type jwtPayload struct {
	Allow []string
}

func APISecret(keystore types.KeyStore, lr repo.LockedRepo) (*APIAlg, error) {
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

	return (*APIAlg)(jwt.NewHS256(key.PrivateKey)), nil
}

