package lf3

import (
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type Config struct {
	// BaseNetworkName is the base from which dynamic network names are defined and is usually
	// the name of the network defined by the static manifest. This must be set correctly or,
	// e.g., pubsub topic filters won't work correctly.
	BaseNetworkName gpbft.NetworkName
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

// NewConfig creates a new F3 config based on the node's build parameters and the passed network
// name.
func NewConfig(nn dtypes.NetworkName) *Config {
	// Use "filecoin" as the network name on mainnet, otherwise use the network name. Yes,
	// mainnet is called testnetnet in state.
	if nn == "testnetnet" {
		nn = "filecoin"
	}
	c := &Config{
		BaseNetworkName: gpbft.NetworkName(nn),
		StaticManifest:  buildconstants.F3Manifest(),
	}

	return c
}
