package impl

import (
	"context"
	"io/ioutil"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilder *sectorbuilder.SectorBuilder
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) (uint64, error) {
	data := make([]byte, 1024)
	fi, err := ioutil.TempFile("", "lotus-garbage")
	if err != nil {
		return 0, err
	}

	if _, err := fi.Write(data); err != nil {
		return 0, err
	}
	fi.Close()

	sectorId, err := sm.SectorBuilder.AddPiece("foo", 1024, fi.Name())
	if err != nil {
		return 0, err
	}

	return sectorId, err
}

var _ api.StorageMiner = &StorageMinerAPI{}
