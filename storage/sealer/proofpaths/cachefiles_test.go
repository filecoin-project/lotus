package proofpaths

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestSDRLayersDefined(t *testing.T) {
	for proof := range abi.SealProofInfos {
		// TODO: Drop after feat/nv21 is merged in (that is, when SynthPoRep changes land)
		if proof >= abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep && proof <= abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep {
			continue
		}
		_, err := SDRLayers(proof)
		require.NoError(t, err)
	}
}
