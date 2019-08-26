package retrieval

import blocks "github.com/ipfs/go-block-format"

type BlockVerifier interface {
	Verify(blocks.Block) (internal bool, err error)
}

// TODO: BalancedUnixFs0Verifier

type OptimisticVerifier struct {
}

func (o *OptimisticVerifier) Verify(blocks.Block) (bool, error) {
	// It's probably fine
	return false, nil
}

var _ BlockVerifier = &OptimisticVerifier{}
