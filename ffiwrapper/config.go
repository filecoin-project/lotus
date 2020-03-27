package ffiwrapper

import (
	"fmt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type Config struct {
	SealProofType abi.RegisteredProof
	PoStProofType abi.RegisteredProof

	_ struct{} // guard against nameless init
}

func sizeFromConfig(cfg Config) (abi.SectorSize, error) {
	if cfg.SealProofType == abi.RegisteredProof(0) {
		return abi.SectorSize(0), xerrors.New("must specify a seal proof type from abi.RegisteredProof")
	}

	if cfg.PoStProofType == abi.RegisteredProof(0) {
		return abi.SectorSize(0), xerrors.New("must specify a PoSt proof type from abi.RegisteredProof")
	}

	s1, err := SectorSizeForRegisteredProof(cfg.SealProofType)
	if err != nil {
		return abi.SectorSize(0), err
	}

	s2, err := SectorSizeForRegisteredProof(cfg.PoStProofType)
	if err != nil {
		return abi.SectorSize(0), err
	}

	if s1 != s2 {
		return abi.SectorSize(0), xerrors.Errorf("seal sector size %d does not equal PoSt sector size %d", s1, s2)
	}

	return s1, nil
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
