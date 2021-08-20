package kit

import (
	"testing"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// EnableLargeSectors enables 512MiB sectors. This is useful in combination with
// mock proofs, for testing larger transfers.
func EnableLargeSectors(t *testing.T) {
	policy.SetSupportedProofTypes(
		abi.RegisteredSealProof_StackedDrg2KiBV1,
		abi.RegisteredSealProof_StackedDrg512MiBV1, // <== here
	)
	t.Cleanup(func() { // reset when done.
		policy.SetSupportedProofTypes(
			abi.RegisteredSealProof_StackedDrg2KiBV1,
		)
	})
}
