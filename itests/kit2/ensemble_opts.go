package kit2

import (
	"time"

	"github.com/filecoin-project/go-state-types/abi"
)

type EnsembleOpt func(opts *ensembleOpts) error

type ensembleOpts struct {
	pastOffset time.Duration
	proofType  abi.RegisteredSealProof
	mockProofs bool
}

var DefaultEnsembleOpts = ensembleOpts{
	pastOffset: 10000 * time.Second,
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
