package sectorutil

import (
	"fmt"
	"github.com/filecoin-project/go-sectorbuilder"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

func ParseSectorID(baseName string) (abi.SectorID, error) {
	var n abi.SectorNumber
	var mid abi.ActorID
	read, err := fmt.Sscanf(baseName, "s-t0%d-%d", &mid, &n)
	if err != nil {
		return abi.SectorID{}, xerrors.Errorf(": %w", err)
	}

	if read != 2 {
		return abi.SectorID{}, xerrors.Errorf("parseSectorID expected to scan 2 values, got %d", read)
	}

	return abi.SectorID{
		Miner:  mid,
		Number: n,
	}, nil
}

func SectorName(sid abi.SectorID) string {
	return fmt.Sprintf("s-t0%d-%d", sid.Miner, sid.Number)
}

func PathByType(sps sectorbuilder.SectorPaths, fileType sectorbuilder.SectorFileType) string {
	switch fileType {
	case sectorbuilder.FTUnsealed:
		return sps.Unsealed
	case sectorbuilder.FTSealed:
		return sps.Sealed
	case sectorbuilder.FTCache:
		return sps.Cache
	}

	panic("requested unknown path type")
}

func SetPathByType(sps *sectorbuilder.SectorPaths, fileType sectorbuilder.SectorFileType, p string) {
	switch fileType {
	case sectorbuilder.FTUnsealed:
		sps.Unsealed = p
	case sectorbuilder.FTSealed:
		sps.Sealed = p
	case sectorbuilder.FTCache:
		sps.Cache = p
	}
}
