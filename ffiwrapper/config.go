package ffiwrapper

import (
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

	return cfg.SealProofType.SectorSize()
}

func SealProofTypeFromSectorSize(ssize abi.SectorSize) (abi.RegisteredProof, error) {
	switch ssize {
	case 2 << 10:
		return abi.RegisteredProof_StackedDRG2KiBSeal, nil
	case 8 << 20:
		return abi.RegisteredProof_StackedDRG8MiBSeal, nil
	case 512 << 20:
		return abi.RegisteredProof_StackedDRG512MiBSeal, nil
	case 32 << 30:
		return abi.RegisteredProof_StackedDRG32GiBSeal, nil
	case 64 << 30:
		return abi.RegisteredProof_StackedDRG64GiBSeal, nil
	default:
		return 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
	}
}
