package ffiwrapper

import (
	"fmt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type Config struct {
	SealProofType abi.RegisteredProof

	_ struct{} // guard against nameless init
}

func sizeFromConfig(cfg Config) (abi.SectorSize, error) {
	if cfg.SealProofType == abi.RegisteredProof(0) {
		return abi.SectorSize(0), xerrors.New("must specify a seal proof type from abi.RegisteredProof")
	}


	return SectorSizeForRegisteredProof(cfg.SealProofType)
}

// TODO: remove this method after implementing it along side the registered proofs and importing it from there.
func SectorSizeForRegisteredProof(p abi.RegisteredProof) (abi.SectorSize, error) {
	switch p {
	case abi.RegisteredProof_StackedDRG32GiBSeal, abi.RegisteredProof_StackedDRG32GiBPoSt:
		return 32 << 30, nil
	case abi.RegisteredProof_StackedDRG2KiBSeal, abi.RegisteredProof_StackedDRG2KiBPoSt:
		return 2 << 10, nil
	case abi.RegisteredProof_StackedDRG8MiBSeal, abi.RegisteredProof_StackedDRG8MiBPoSt:
		return 8 << 20, nil
	case abi.RegisteredProof_StackedDRG512MiBSeal, abi.RegisteredProof_StackedDRG512MiBPoSt:
		return 512 << 20, nil
	default:
		return 0, fmt.Errorf("unsupported registered proof %d", p)
	}
}

func ProofTypeFromSectorSize(ssize abi.SectorSize) (abi.RegisteredProof, abi.RegisteredProof, error) {
	switch ssize {
	case 2 << 10:
		return abi.RegisteredProof_StackedDRG2KiBPoSt, abi.RegisteredProof_StackedDRG2KiBSeal, nil
	case 8 << 20:
		return abi.RegisteredProof_StackedDRG8MiBPoSt, abi.RegisteredProof_StackedDRG8MiBSeal, nil
	case 512 << 20:
		return abi.RegisteredProof_StackedDRG512MiBPoSt, abi.RegisteredProof_StackedDRG512MiBSeal, nil
	case 32 << 30:
		return abi.RegisteredProof_StackedDRG32GiBPoSt, abi.RegisteredProof_StackedDRG32GiBSeal, nil
	default:
		return 0, 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
	}
}
