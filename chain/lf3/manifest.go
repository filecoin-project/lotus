package lf3

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/ec"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type headGetter store.ChainStore

func (hg *headGetter) GetHead(context.Context) (ec.TipSet, error) {
	head := (*store.ChainStore)(hg).GetHeaviestTipSet()
	if head == nil {
		return nil, xerrors.New("no heaviest tipset")
	}
	return &f3TipSet{TipSet: head}, nil
}

// Determines the max. number of configuration changes
// that are allowed for the dynamic manifest.
// If the manifest changes more than this number, the F3
// message topic will be filtered
var MaxDynamicManifestChangesAllowed = 1000

func NewManifestProvider(mctx helpers.MetricsCtx, config *Config, cs *store.ChainStore, ps *pubsub.PubSub, mds dtypes.MetadataDS) (prov manifest.ManifestProvider, err error) {
	if config.DynamicManifestProvider == "" || !build.IsF3PassiveTestingEnabled() {
		if config.StaticManifest == nil {
			return manifest.NoopManifestProvider{}, nil
		}
		return manifest.NewStaticManifestProvider(config.StaticManifest)
	}

	opts := []manifest.DynamicManifestProviderOption{
		manifest.DynamicManifestProviderWithDatastore(
			namespace.Wrap(mds, datastore.NewKey("/f3-dynamic-manifest")),
		),
	}

	if config.StaticManifest != nil {
		opts = append(opts,
			manifest.DynamicManifestProviderWithInitialManifest(config.StaticManifest),
		)
	}

	if config.AllowDynamicFinalize {
		log.Error("dynamic F3 manifests are allowed to finalize tipsets, do not enable this in production!")
	}

	networkNameBase := config.BaseNetworkName + "/"
	filter := func(m *manifest.Manifest) error {
		if m.EC.Finalize {
			if !config.AllowDynamicFinalize {
				return fmt.Errorf("refusing dynamic manifest that finalizes tipsets")
			}
			log.Error("WARNING: loading a dynamic F3 manifest that will finalize new tipsets")
		}
		if !strings.HasPrefix(string(m.NetworkName), string(networkNameBase)) {
			return fmt.Errorf(
				"refusing dynamic manifest with network name %q, must start with %q",
				m.NetworkName,
				networkNameBase,
			)
		}
		return nil
	}
	opts = append(opts,
		manifest.DynamicManifestProviderWithFilter(filter),
	)

	prov, err = manifest.NewDynamicManifestProvider(ps, config.DynamicManifestProvider, opts...)
	if err != nil {
		return nil, err
	}
	if config.PrioritizeStaticManifest && config.StaticManifest != nil {
		prov, err = manifest.NewFusingManifestProvider(mctx,
			(*headGetter)(cs), prov, config.StaticManifest)
	}
	return prov, err
}
