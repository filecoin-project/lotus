package lf3

import (
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func NewManifestProvider(nn dtypes.NetworkName, ps *pubsub.PubSub) manifest.ManifestProvider {
	m := manifest.LocalDevnetManifest()
	m.NetworkName = gpbft.NetworkName(nn)
	m.ECDelayMultiplier = 2.
	m.ECPeriod = time.Duration(build.BlockDelaySecs) * time.Second
	m.BootstrapEpoch = int64(buildconstants.F3BootstrapEpoch)
	m.ECFinality = int64(build.Finality)
	m.CommitteeLookback = 5

	switch manifestServerID, err := peer.Decode(buildconstants.ManifestServerID); {
	case err != nil:
		log.Warnw("Cannot decode F3 manifest sever identity; falling back on static manifest provider", "err", err)
		return manifest.NewStaticManifestProvider(m)
	default:
		return manifest.NewDynamicManifestProvider(m, ps, manifestServerID)
	}
}
