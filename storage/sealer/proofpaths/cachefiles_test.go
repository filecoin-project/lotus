package proofpaths

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestSDRLayersDefined(t *testing.T) {
	for proof := range abi.SealProofInfos {
		_, err := SDRLayers(proof)
		require.NoError(t, err)
	}
}
