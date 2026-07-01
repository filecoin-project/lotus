package kit

import (
	"errors"
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

type EnsembleOpt func(opts *ensembleOpts) error

type proofMode int

const (
	proofModeUnset proofMode = iota
	proofModeMock
	proofModeReal
)

type genesisAccount struct {
	key            *key.Key
	initialBalance abi.TokenAmount
}

type ensembleOpts struct {
	pastOffset         time.Duration
	verifiedRoot       genesisAccount
	accounts           []genesisAccount
	proofMode          proofMode
	mockProofs         bool
	realStorageManager bool
	networkName        string

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
func MockProofs() EnsembleOpt {
	return func(opts *ensembleOpts) error {
		if opts.proofMode == proofModeReal {
			return errors.New("proof mode already set to real proofs; use either kit.MockProofs() or kit.RealProofs()")
		}
		opts.proofMode = proofModeMock
		opts.mockProofs = true
		// since we're using mock proofs, we don't need to download
		// proof parameters
		build.DisableBuiltinAssets = true
		return nil
	}
}

// RealProofs activates real proofs for the entire ensemble.
// CI scans for this marker when deciding whether to download proof parameters
// for an itest.
func RealProofs() EnsembleOpt {
	return func(opts *ensembleOpts) error {
		if opts.proofMode == proofModeMock {
			return errors.New("proof mode already set to mock proofs; use either kit.MockProofs() or kit.RealProofs()")
		}
		opts.proofMode = proofModeReal
		opts.mockProofs = false
		build.DisableBuiltinAssets = false
		return nil
	}
}

// WithRealStorageManager keeps the real storage manager wired when using
// MockProofs. This is useful for tests that exercise the storage-manager API
// surface without needing real cryptographic proofs.
func WithRealStorageManager() EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.realStorageManager = true
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
