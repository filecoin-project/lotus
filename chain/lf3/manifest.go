package lf3

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

func NewManifestProvider(mctx helpers.MetricsCtx, config *Config) (prov manifest.ManifestProvider, err error) {
	if config.StaticManifest == nil {
		return manifest.NoopManifestProvider{}, nil
	}
	if config.StaticManifest != nil && build.IsF3EpochActivationDisabled(config.StaticManifest.BootstrapEpoch) {
		log.Warnf("F3 activation disabled by environment configuration for bootstrap epoch %d", config.StaticManifest.BootstrapEpoch)
		return manifest.NoopManifestProvider{}, nil
	}
	smp, err := manifest.NewStaticManifestProvider(config.StaticManifest)
	if err != nil {
		return nil, xerrors.Errorf("creating static manifest provider: %w", err)
	}
	return smp, err
}
