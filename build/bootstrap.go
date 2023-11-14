package build

import (
	"context"
	"embed"
	"path"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/lotus/lib/addrutil"
)

//go:embed bootstrap
var bootstrapfs embed.FS

func BuiltinBootstrap() ([]peer.AddrInfo, error) {
	if DisableBuiltinAssets {
		return nil, nil
	}
	if BootstrappersFile != "" {
		spi, err := bootstrapfs.ReadFile(path.Join("bootstrap", BootstrappersFile))
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
