//go:build cgo

package ffiwrapper

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var ProofProver = proofProver{}

var _ storiface.Prover = ProofProver

type proofProver struct{}

func (v proofProver) AggregateSealProofs(aggregateInfo proof.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}
