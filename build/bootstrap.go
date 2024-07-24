package build

import (
	"context"
	"embed"
	"os"
	"path"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/lib/addrutil"
)

//go:embed bootstrap
var bootstrapfs embed.FS

func BuiltinBootstrap() ([]peer.AddrInfo, error) {
	if DisableBuiltinAssets {
		return nil, nil
	}
	if bootstrappers, found := os.LookupEnv(buildconstants.BootstrappersOverrideEnvVarKey); found {
		log.Infof("Using bootstrap nodes overridden by environment variable %s", buildconstants.BootstrappersOverrideEnvVarKey)
		return addrutil.ParseAddresses(context.TODO(), strings.Split(strings.TrimSpace(bootstrappers), ","))
	}
	if buildconstants.BootstrappersFile != "" {
		spi, err := bootstrapfs.ReadFile(path.Join("bootstrap", buildconstants.BootstrappersFile))
		if err != nil {
			return nil, err
		}
		if len(spi) == 0 {
			return nil, nil
		}

		return addrutil.ParseAddresses(context.TODO(), strings.Split(strings.TrimSpace(string(spi)), "\n"))
	}

	return nil, nil
}
