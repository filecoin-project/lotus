package proofpaths

import (
	"fmt"
	"regexp"

	"github.com/filecoin-project/go-state-types/abi"
)

const dataFilePrefix = "sc-02-data-"
const TreeDName = dataFilePrefix + "tree-d.dat"

const TreeRLastPrefix = dataFilePrefix + "tree-r-last-"
const TreeCPrefix = dataFilePrefix + "tree-c-"

func IsFileTreeD(baseName string) bool {
	return baseName == TreeDName
}

func IsFileTreeRLast(baseName string) bool {
	// TreeRLastPrefix<int>.dat
	reg := fmt.Sprintf(`^%s\d+\.dat$`, TreeRLastPrefix)
	return regexp.MustCompile(reg).MatchString(baseName)
}

func IsFileTreeC(baseName string) bool {
	// TreeCPrefix<int>.dat
	reg := fmt.Sprintf(`^%s\d+\.dat$`, TreeCPrefix)
	return regexp.MustCompile(reg).MatchString(baseName)
}

func IsTreeFile(baseName string) bool {
	return IsFileTreeD(baseName) || IsFileTreeRLast(baseName) || IsFileTreeC(baseName)
}

func LayerFileName(layer int) string {
	return fmt.Sprintf("%slayer-%d.dat", dataFilePrefix, layer)
}

func SDRLayers(spt abi.RegisteredSealProof) (int, error) {
	switch spt {
	case abi.RegisteredSealProof_StackedDrg2KiBV1, abi.RegisteredSealProof_StackedDrg2KiBV1_1, abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep, abi.RegisteredSealProof_StackedDrg2KiBV1_2_Feat_NiPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg8MiBV1, abi.RegisteredSealProof_StackedDrg8MiBV1_1, abi.RegisteredSealProof_StackedDrg8MiBV1_1_Feat_SyntheticPoRep, abi.RegisteredSealProof_StackedDrg8MiBV1_2_Feat_NiPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg512MiBV1, abi.RegisteredSealProof_StackedDrg512MiBV1_1, abi.RegisteredSealProof_StackedDrg512MiBV1_1_Feat_SyntheticPoRep, abi.RegisteredSealProof_StackedDrg512MiBV1_2_Feat_NiPoRep:
		return 2, nil
	case abi.RegisteredSealProof_StackedDrg32GiBV1, abi.RegisteredSealProof_StackedDrg32GiBV1_1, abi.RegisteredSealProof_StackedDrg32GiBV1_1_Feat_SyntheticPoRep, abi.RegisteredSealProof_StackedDrg32GiBV1_2_Feat_NiPoRep:
		return 11, nil
	case abi.RegisteredSealProof_StackedDrg64GiBV1, abi.RegisteredSealProof_StackedDrg64GiBV1_1, abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep, abi.RegisteredSealProof_StackedDrg64GiBV1_2_Feat_NiPoRep:
		return 11, nil
	default:
		return 0, fmt.Errorf("unsupported proof type: %v", spt)
	}
}

func IsTreeRCFile(baseName string) bool {
	return IsFileTreeRLast(baseName) || IsFileTreeC(baseName)
}

func IsTreeDFile(baseName string) bool {
	return IsFileTreeD(baseName)
}
