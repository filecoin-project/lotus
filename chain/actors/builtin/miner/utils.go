package miner

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
)

type SealProofVariant int

const (
	SealProofVariant_Standard SealProofVariant = iota
	SealProofVariant_Synthetic
	SealProofVariant_NonInteractive
)

var MinSyntheticPoRepVersion = network.Version21
var MinNonInteractivePoRepVersion = network.Version23

func AllPartSectors(mas State, sget func(Partition) (bitfield.BitField, error)) (bitfield.BitField, error) {
	var parts []bitfield.BitField

	err := mas.ForEachDeadline(func(dlidx uint64, dl Deadline) error {
		return dl.ForEachPartition(func(partidx uint64, part Partition) error {
			s, err := sget(part)
			if err != nil {
				return xerrors.Errorf("getting sector list (dl: %d, part %d): %w", dlidx, partidx, err)
			}

			parts = append(parts, s)
			return nil
		})
	})
	if err != nil {
		return bitfield.BitField{}, err
	}

	return bitfield.MultiMerge(parts...)
}

// SealProofTypeFromSectorSize returns preferred seal proof type for creating
// new miner actors and new sectors
func SealProofTypeFromSectorSize(ssize abi.SectorSize, nv network.Version, variant SealProofVariant) (abi.RegisteredSealProof, error) {
	switch variant {
	case SealProofVariant_Synthetic:
		if nv < MinSyntheticPoRepVersion {
			return 0, xerrors.Errorf("synthetic proofs are not supported on network version %d", nv)
		}
	case SealProofVariant_NonInteractive:
		if nv < MinNonInteractivePoRepVersion {
			return 0, xerrors.Errorf("non-interactive proofs are not supported on network version %d", nv)
		}
	}

	switch {
	case nv < network.Version7:
		switch ssize {
		case 2 << 10:
			return abi.RegisteredSealProof_StackedDrg2KiBV1, nil
		case 8 << 20:
			return abi.RegisteredSealProof_StackedDrg8MiBV1, nil
		case 512 << 20:
			return abi.RegisteredSealProof_StackedDrg512MiBV1, nil
		case 32 << 30:
			return abi.RegisteredSealProof_StackedDrg32GiBV1, nil
		case 64 << 30:
			return abi.RegisteredSealProof_StackedDrg64GiBV1, nil
		default:
			return 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
		}
	case nv >= network.Version7:
		var v abi.RegisteredSealProof
		switch ssize {
		case 2 << 10:
			v = abi.RegisteredSealProof_StackedDrg2KiBV1_1
		case 8 << 20:
			v = abi.RegisteredSealProof_StackedDrg8MiBV1_1
		case 512 << 20:
			v = abi.RegisteredSealProof_StackedDrg512MiBV1_1
		case 32 << 30:
			v = abi.RegisteredSealProof_StackedDrg32GiBV1_1
		case 64 << 30:
			v = abi.RegisteredSealProof_StackedDrg64GiBV1_1
		default:
			return 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
		}

		switch variant {
		case SealProofVariant_Synthetic:
			return toSynthetic(v)
		case SealProofVariant_NonInteractive:
			return toNonInteractive(v)
		}
		return v, nil
	}

	return 0, xerrors.Errorf("unsupported network version")
}

func toSynthetic(in abi.RegisteredSealProof) (abi.RegisteredSealProof, error) {
	switch in {
	case abi.RegisteredSealProof_StackedDrg2KiBV1_1:
		return abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredSealProof_StackedDrg8MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg8MiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredSealProof_StackedDrg512MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg512MiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredSealProof_StackedDrg32GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg32GiBV1_1_Feat_SyntheticPoRep, nil
	case abi.RegisteredSealProof_StackedDrg64GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep, nil
	default:
		return 0, xerrors.Errorf("unsupported conversion to synthetic: %v", in)
	}
}

func toNonInteractive(in abi.RegisteredSealProof) (abi.RegisteredSealProof, error) {
	switch in {
	case abi.RegisteredSealProof_StackedDrg2KiBV1_1:
		return abi.RegisteredSealProof_StackedDrg2KiBV1_2_Feat_NiPoRep, nil
	case abi.RegisteredSealProof_StackedDrg8MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg8MiBV1_2_Feat_NiPoRep, nil
	case abi.RegisteredSealProof_StackedDrg512MiBV1_1:
		return abi.RegisteredSealProof_StackedDrg512MiBV1_2_Feat_NiPoRep, nil
	case abi.RegisteredSealProof_StackedDrg32GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg32GiBV1_2_Feat_NiPoRep, nil
	case abi.RegisteredSealProof_StackedDrg64GiBV1_1:
		return abi.RegisteredSealProof_StackedDrg64GiBV1_2_Feat_NiPoRep, nil
	default:
		return 0, xerrors.Errorf("unsupported conversion to non-interactive: %v", in)
	}
}

// WindowPoStProofTypeFromSectorSize returns preferred post proof type for creating
// new miner actors and new sectors
func WindowPoStProofTypeFromSectorSize(ssize abi.SectorSize, nv network.Version) (abi.RegisteredPoStProof, error) {
	switch {
	case nv < network.Version19:
		switch ssize {
		case 2 << 10:
			return abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, nil
		case 8 << 20:
			return abi.RegisteredPoStProof_StackedDrgWindow8MiBV1, nil
		case 512 << 20:
			return abi.RegisteredPoStProof_StackedDrgWindow512MiBV1, nil
		case 32 << 30:
			return abi.RegisteredPoStProof_StackedDrgWindow32GiBV1, nil
		case 64 << 30:
			return abi.RegisteredPoStProof_StackedDrgWindow64GiBV1, nil
		default:
			return 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
		}
	case nv >= network.Version19:
		switch ssize {
		case 2 << 10:
			return abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1, nil
		case 8 << 20:
			return abi.RegisteredPoStProof_StackedDrgWindow8MiBV1_1, nil
		case 512 << 20:
			return abi.RegisteredPoStProof_StackedDrgWindow512MiBV1_1, nil
		case 32 << 30:
			return abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1, nil
		case 64 << 30:
			return abi.RegisteredPoStProof_StackedDrgWindow64GiBV1_1, nil
		default:
			return 0, xerrors.Errorf("unsupported sector size for miner: %v", ssize)
		}
	}
	return 0, xerrors.Errorf("unsupported network version")
}
