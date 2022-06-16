//go:build cgo
// +build cgo

package ffiwrapper

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/proof"
)

var ProofProver = proofProver{}

var _ Prover = ProofProver

type proofProver struct{}

func (v proofProver) AggregateSealProofs(aggregateInfo proof.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}
