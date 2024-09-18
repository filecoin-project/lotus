package lf3

import (
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Determines the max. number of configuration changes
// that are allowed for the dynamic manifest.
// If the manifest changes more than this number, the F3
// message topic will be filtered
var MaxDynamicManifestChangesAllowed = 1000

type ManifestOut struct {
	fx.Out

	ManifestProvider manifest.ManifestProvider
	PubsubTopics     []string `group:"pubsub-topic,flatten"`
}

func NewManifestProvider(config *Config, ps *pubsub.PubSub, mds dtypes.MetadataDS) manifest.ManifestProvider {
	if config.DynamicManifestProvider == "" {
		return manifest.NewStaticManifestProvider(config.InitialManifest)
	}
	ds := namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest"))
	return manifest.NewDynamicManifestProvider(config.InitialManifest, ds, ps, config.DynamicManifestProvider)
}
