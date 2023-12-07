package proofpaths

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
)

var dataFilePrefix = "sc-02-data-"

func LayerFileName(layer int) string {
	return fmt.Sprintf("%slayer-%d.dat", dataFilePrefix, layer)
}

func SDRLayers(spt abi.RegisteredSealProof) (int, error) {
	switch spt {
	case abi.RegisteredSealProof_StackedDrg2KiBV1, abi.RegisteredSealProof_StackedDrg2KiBV1_1, abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg8MiBV1, abi.RegisteredSealProof_StackedDrg8MiBV1_1, abi.RegisteredSealProof_StackedDrg8MiBV1_1_Feat_SyntheticPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg512MiBV1, abi.RegisteredSealProof_StackedDrg512MiBV1_1, abi.RegisteredSealProof_StackedDrg512MiBV1_1_Feat_SyntheticPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg32GiBV1, abi.RegisteredSealProof_StackedDrg32GiBV1_1, abi.RegisteredSealProof_StackedDrg32GiBV1_1_Feat_SyntheticPoRep:
		return 11, nil
	case abi.RegisteredSealProof_StackedDrg64GiBV1, abi.RegisteredSealProof_StackedDrg64GiBV1_1, abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep:
		return 11, nil
	default:
		return 0, fmt.Errorf("unsupported proof type: %v", spt)
	}
}
