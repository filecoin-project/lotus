package lf3

import (
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func NewManifestProvider(nn dtypes.NetworkName, cs *store.ChainStore, sm *stmgr.StateManager, ps *pubsub.PubSub) manifest.ManifestProvider {
	m := NewManifest(nn)

	ec := &ecWrapper{
		ChainStore:   cs,
		StateManager: sm,
	}

	switch manifestServerID, err := peer.Decode(build.ManifestServerID); {
	case err != nil:
		log.Warnw("Cannot decode F3 manifest sever identity; falling back on static manifest provider", "err", err)
		return manifest.NewStaticManifestProvider(m)
	default:
		return manifest.NewDynamicManifestProvider(m, ps, ec, manifestServerID)
	}
}

func NewManifest(nn dtypes.NetworkName) *manifest.Manifest {
	m := manifest.LocalDevnetManifest()
	m.NetworkName = gpbft.NetworkName(nn)
	m.ECPeriod = build.F3BlockDelay
	m.ECDelayMultiplier = 2.
	m.BootstrapEpoch = int64(build.F3BootstrapEpoch)
	m.ECFinality = int64(build.F3Finality)
	m.CommiteeLookback = 5

	return m
}
