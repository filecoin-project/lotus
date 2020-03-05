package advmgr

import (
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"
)

type readonlyProvider struct {
	miner abi.ActorID
	stor  *storage
}

func (l *readonlyProvider) AcquireSectorNumber() (abi.SectorNumber, error) {
	return 0, xerrors.New("read-only provider")
}

func (l *readonlyProvider) FinalizeSector(abi.SectorNumber) error {
	return xerrors.New("read-only provider")
}

func (l *readonlyProvider) AcquireSector(id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if allocate != 0 {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	return l.stor.acquireSector(l.miner, id, existing, allocate, sealing)
}
