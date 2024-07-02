package lf3

import (
	"time"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

func NewManifestProvider(nn dtypes.NetworkName, cs *store.ChainStore, sm *stmgr.StateManager, ps *pubsub.PubSub) manifest.ManifestProvider {
	m := manifest.LocalDevnetManifest()
	m.NetworkName = gpbft.NetworkName(nn)
	m.ECDelay = 2 * time.Duration(build.BlockDelaySecs) * time.Second
	m.ECPeriod = m.ECDelay
	m.BootstrapEpoch = int64(build.F3BootstrapEpoch)
	m.ECFinality = int64(build.Finality)
	m.CommiteeLookback = 5

	ec := &ecWrapper{
		ChainStore:   cs,
		StateManager: sm,
	}

	manifestServerID := build.ManifestServerID
	if build.ManifestServerID == peer.ID("") {
		return manifest.NewDynamicManifestProvider(m, ps, ec, manifestServerID)
	} else {
		return manifest.NewStaticManifestProvider(m)
	}

}
