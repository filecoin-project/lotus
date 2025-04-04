package kit

import (
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

type EnsembleOpt func(opts *ensembleOpts) error

type genesisAccount struct {
	key            *key.Key
	initialBalance abi.TokenAmount
}

type ensembleOpts struct {
	pastOffset   time.Duration
	verifiedRoot genesisAccount
	accounts     []genesisAccount
	mockProofs   bool
	networkName  string

	upgradeSchedule stmgr.UpgradeSchedule
}

var DefaultEnsembleOpts = ensembleOpts{
	pastOffset: 10000000 * time.Second, // time sufficiently in the past to trigger catch-up mining.
	upgradeSchedule: stmgr.UpgradeSchedule{{
		Height:  -1,
		Network: buildconstants.TestNetworkVersion,
	}},
	networkName: "testing", // to match the network bundle
}

// MockProofs activates mock proofs for the entire ensemble.
func MockProofs(e ...bool) EnsembleOpt {
	if len(e) > 0 && !e[0] {
		return func(opts *ensembleOpts) error {
			return nil
		}
	}

	return func(opts *ensembleOpts) error {
		opts.mockProofs = true
		// since we're using mock proofs, we don't need to download
		// proof parameters
		build.DisableBuiltinAssets = true
		return nil
	}
}

// RootVerifier specifies the key to be enlisted as the verified registry root,
// as well as the initial balance to be attributed during genesis.
func RootVerifier(key *key.Key, balance abi.TokenAmount) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.verifiedRoot.key = key
		opts.verifiedRoot.initialBalance = balance
		return nil
	}
}

// NetworkName sets the network name for the ensemble in the genesis template.
func NetworkName(name string) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.networkName = name
		return nil
	}
}

// Account sets up an account at genesis with the specified key and balance.
func Account(key *key.Key, balance abi.TokenAmount) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.accounts = append(opts.accounts, genesisAccount{
			key:            key,
			initialBalance: balance,
		})
		return nil
	}
}
