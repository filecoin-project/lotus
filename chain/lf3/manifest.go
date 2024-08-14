package lf3

import (
	"fmt"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
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

func NewManifestProvider(provider peer.ID) any {
	if provider == "" {
		return manifest.NewStaticManifestProvider
	}
	return func(m *manifest.Manifest, ps *pubsub.PubSub, mds dtypes.MetadataDS) ManifestOut {
		var out ManifestOut
		ds := namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest"))
		f3BaseTopicName := manifest.PubSubTopicFromNetworkName(gpbft.NetworkName(m.NetworkName))
		out.ManifestProvider = manifest.NewDynamicManifestProvider(m, ds, ps, provider)
		out.PubsubTopics = append(out.PubsubTopics, manifest.ManifestPubSubTopicName)

		// allow dynamic manifest topic and the new topic names after a reconfiguration.
		// Note: This is pretty ugly, but I tried to use a regex subscription filter
		// as the commented code below, but unfortunately it overwrites previous filters. A simple fix would
		// be to allow combining several topic filters, but for now this works.
		//
		// 	pattern := fmt.Sprintf(`^\/f3\/%s\/0\.0\.1\/?[0-9]*$`, in.Nn)
		// 	rx, err := regexp.Compile(pattern)
		// 	if err != nil {
		// 		return nil, xerrors.Errorf("failed to compile manifest topic regex: %w", err)
		// 	}
		// 	options = append(options,
		// 		pubsub.WithSubscriptionFilter(
		// 			pubsub.WrapLimitSubscriptionFilter(
		// 				pubsub.NewRegexpSubscriptionFilter(rx),
		// 				100)))
		for i := 0; i < MaxDynamicManifestChangesAllowed; i++ {
			out.PubsubTopics = append(out.PubsubTopics, fmt.Sprintf("%s/%d", f3BaseTopicName, i))
		}

		return out
	}
}

func NewManifest(nn dtypes.NetworkName) *manifest.Manifest {
	m := manifest.LocalDevnetManifest()
	m.NetworkName = gpbft.NetworkName(nn)
	m.EC.Period = time.Duration(buildconstants.BlockDelaySecs) * time.Second
	if buildconstants.F3BootstrapEpoch < 0 {
		// if unset, set to a sane default so we don't get scary logs and pause.
		m.BootstrapEpoch = 2 * int64(policy.ChainFinality)
		m.Pause = true
	} else {
		m.BootstrapEpoch = int64(buildconstants.F3BootstrapEpoch)
	}
	m.EC.Finality = int64(policy.ChainFinality)
	m.CommitteeLookback = 5

	// TODO: We're forcing this to start paused for now. We need to remove this for the final
	// mainnet launch.
	m.Pause = true
	return m
}
