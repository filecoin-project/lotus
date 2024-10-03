package lf3

import (
	"fmt"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Determines the max. number of configuration changes
// that are allowed for the dynamic manifest.
// If the manifest changes more than this number, the F3
// message topic will be filtered
var MaxDynamicManifestChangesAllowed = 1000

func NewManifestProvider(config *Config, ps *pubsub.PubSub, mds dtypes.MetadataDS) (manifest.ManifestProvider, error) {
	if config.DynamicManifestProvider == "" {
		return manifest.NewStaticManifestProvider(config.InitialManifest)
	}

	primaryNetworkName := config.InitialManifest.NetworkName
	filter := func(m *manifest.Manifest) error {
		if m.EC.Finalize {
			return fmt.Errorf("refusing dynamic manifest that finalizes tipsets")
		}
		if m.NetworkName == primaryNetworkName {
			return fmt.Errorf(
				"refusing dynamic manifest with network name %q that clashes with initial manifest",
				primaryNetworkName,
			)
		}
		return nil
	}
	ds := namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest"))
	return manifest.NewDynamicManifestProvider(ps, config.DynamicManifestProvider,
		manifest.DynamicManifestProviderWithInitialManifest(config.InitialManifest),
		manifest.DynamicManifestProviderWithDatastore(ds),
		manifest.DynamicManifestProviderWithFilter(filter),
	)
}
