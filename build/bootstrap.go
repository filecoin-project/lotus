package build

import (
	"context"
	"os"
	"strconv"

	"github.com/filecoin-project/lotus/lib/addrutil"

	"github.com/libp2p/go-libp2p-core/peer"
)

func BuiltinBootstrap() ([]peer.AddrInfo, error) {
	if DisableBuiltinAssets {
		return nil, nil
	}
	return addrutil.ParseAddresses(context.TODO(), activeNetworkParams.Bootstrappers)
}

func BootstrapPeerThreshold() int {
	threshold := activeNetworkParams.Config.BootstrapPeerThreshold
	if ev, ok := os.LookupEnv("LOTUS_SYNC_BOOTSTRAP_PEERS"); ok {
		envthreshold, err := strconv.Atoi(ev)
		if err != nil {
			log.Errorf("failed to parse 'LOTUS_SYNC_BOOTSTRAP_PEERS' env var: %s", err)
		} else {
			threshold = envthreshold
		}
	}
	return threshold
}
