package lf3

import (
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func NewManifestProvider(nn dtypes.NetworkName, ps *pubsub.PubSub, mds dtypes.MetadataDS) manifest.ManifestProvider {
	m := manifest.LocalDevnetManifest()
	m.NetworkName = gpbft.NetworkName(nn)
	m.EC.DelayMultiplier = 2.
	m.EC.Period = time.Duration(build.BlockDelaySecs) * time.Second
	if build.F3BootstrapEpoch < 0 {
		// if unset, set to a sane default so we don't get scary logs and pause.
		m.BootstrapEpoch = 2 * int64(policy.ChainFinality)
		m.Pause = true
	} else {
		m.BootstrapEpoch = int64(build.F3BootstrapEpoch)
	}
	m.EC.Finality = int64(policy.ChainFinality)
	m.CommitteeLookback = 5

	// TODO: We're forcing this to start paused for now. We need to remove this for the final
	// mainnet launch.
	m.Pause = true

	switch manifestServerID, err := peer.Decode(build.ManifestServerID); {
	case err != nil:
		log.Warnw("Cannot decode F3 manifest sever identity; falling back on static manifest provider", "err", err)
		return manifest.NewStaticManifestProvider(m)
	default:
		ds := namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest"))
		return manifest.NewDynamicManifestProvider(m, ds, ps, manifestServerID)
	}
}
