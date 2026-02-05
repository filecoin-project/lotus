package lf3

import (
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type Config struct {
	// StaticManifest this instance's default manifest absent any dynamic manifests. Also see
	// PrioritizeStaticManifest.
	StaticManifest *manifest.Manifest
}

// NewManifest constructs a sane F3 manifest based on the passed parameters. This function does not
// look at and/or depend on the nodes build params, etc.
func NewManifest(
	nn gpbft.NetworkName,
	finality, bootstrapEpoch abi.ChainEpoch,
	ecPeriod time.Duration,
	initialPowerTable cid.Cid,
) *manifest.Manifest {
	return &manifest.Manifest{
		ProtocolVersion:   manifest.VersionCapability,
		BootstrapEpoch:    int64(bootstrapEpoch),
		NetworkName:       nn,
		InitialPowerTable: initialPowerTable,
		CommitteeLookback: manifest.DefaultCommitteeLookback,
		CatchUpAlignment:  ecPeriod / 2,
		Gpbft:             manifest.DefaultGpbftConfig,
		EC: manifest.EcConfig{
			Period:                   ecPeriod,
			Finality:                 int64(finality),
			DelayMultiplier:          manifest.DefaultEcConfig.DelayMultiplier,
			BaseDecisionBackoffTable: manifest.DefaultEcConfig.BaseDecisionBackoffTable,
			HeadLookback:             4,
			Finalize:                 true,
		},
		CertificateExchange: manifest.CxConfig{
			ClientRequestTimeout: manifest.DefaultCxConfig.ClientRequestTimeout,
			ServerRequestTimeout: manifest.DefaultCxConfig.ServerRequestTimeout,
			MinimumPollInterval:  ecPeriod,
			MaximumPollInterval:  4 * ecPeriod,
		},
		PubSub:                manifest.DefaultPubSubConfig,
		ChainExchange:         manifest.DefaultChainExchangeConfig,
		PartialMessageManager: manifest.DefaultPartialMessageManagerConfig,
	}
}

// NewConfig creates a new F3 config based on the node's build parameters.
func NewConfig() *Config {
	return &Config{
		StaticManifest: buildconstants.F3Manifest(),
	}
}
