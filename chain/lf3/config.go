package lf3

import (
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type Config struct {
	InitialManifest         *manifest.Manifest
	DynamicManifestProvider peer.ID
	F3ConsensusEnabled      bool
}

func NewConfig(manifestProvider peer.ID, consensusEnabled bool, initialPowerTable cid.Cid) func(dtypes.NetworkName) *Config {
	return func(nn dtypes.NetworkName) *Config {
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
		m.InitialPowerTable = initialPowerTable

		// TODO: We're forcing this to start paused for now. We need to remove this for the final
		// mainnet launch.
		m.Pause = true
		return &Config{
			InitialManifest:         m,
			DynamicManifestProvider: manifestProvider,
			F3ConsensusEnabled:      consensusEnabled,
		}
	}
}
