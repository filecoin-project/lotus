package kit2

import (
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/wallet"
)

type EnsembleOpt func(opts *ensembleOpts) error

type genesisAccount struct {
	key            *wallet.Key
	initialBalance abi.TokenAmount
}

type ensembleOpts struct {
	pastOffset   time.Duration
	proofType    abi.RegisteredSealProof
	verifiedRoot genesisAccount
	accounts     []genesisAccount
	mockProofs   bool
}

var DefaultEnsembleOpts = ensembleOpts{
	pastOffset: 10000000 * time.Second, // time sufficiently in the past to trigger catch-up mining.
	proofType:  abi.RegisteredSealProof_StackedDrg2KiBV1,
}

func ProofType(proofType abi.RegisteredSealProof) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.proofType = proofType
		return nil
	}
}

// MockProofs activates mock proofs for the entire ensemble.
func MockProofs() EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.mockProofs = true
		return nil
	}
}

// RootVerifier specifies the key to be enlisted as the verified registry root,
// as well as the initial balance to be attributed during genesis.
func RootVerifier(key *wallet.Key, balance abi.TokenAmount) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.verifiedRoot.key = key
		opts.verifiedRoot.initialBalance = balance
		return nil
	}
}

// Account sets up an account at genesis with the specified key and balance.
func Account(key *wallet.Key, balance abi.TokenAmount) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.accounts = append(opts.accounts, genesisAccount{
			key:            key,
			initialBalance: balance,
		})
		return nil
	}
}
