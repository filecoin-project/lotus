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

const (
	BootstrappersOverrideEnvVarKey = "LOTUS_P2P_BOOTSTRAPPERS"
)

//go:embed bootstrap
var bootstrapfs embed.FS

func BuiltinBootstrap() ([]peer.AddrInfo, error) {

	// do not connect to anything for itests
	if DisableBuiltinAssets {
		return nil, nil
	}

	var bsList, bsListFin []string

	// envvar always takes precedence
	if bootstrappers, found := os.LookupEnv(BootstrappersOverrideEnvVarKey); found {
		log.Infof("Using bootstrap nodes overridden by environment variable %s", BootstrappersOverrideEnvVarKey)
		bsList = strings.Split(bootstrappers, ",")
	} else if buildconstants.BootstrappersFile != "" {
		spi, err := bootstrapfs.ReadFile(path.Join("bootstrap", buildconstants.BootstrappersFile))
		if err != nil {
			return nil, err
		}
		bsList = strings.Split(string(spi), "\n")
	}

	for _, s := range bsList {
		s = strings.TrimSpace(s)
		if s != "" {
			bsListFin = append(bsListFin, s)
		}
	}

	if len(bsListFin) == 0 {
		return nil, nil
	}

	return addrutil.ParseAddresses(context.TODO(), bsListFin)
}
