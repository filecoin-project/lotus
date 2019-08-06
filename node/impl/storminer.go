package impl

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilder *sectorbuilder.SectorBuilder

	Miner *storage.Miner
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) (uint64, error) {
	maxSize := 1016 // this is the most data we can fit in a 1024 byte sector

	name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))

	sectorId, err := sm.SectorBuilder.AddPiece(ctx, name, uint64(maxSize),
		ioutil.NopCloser(io.LimitReader(rand.New(rand.NewSource(rand.Int63())), int64(maxSize))))
	if err != nil {
		return 0, err
	}

	return sectorId, err
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid uint64) (sectorbuilder.SectorSealingStatus, error) {
	return sm.SectorBuilder.SealStatus(sid)
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsStagedList(context.Context) ([]sectorbuilder.StagedSectorMetadata, error) {
	return sm.SectorBuilder.GetAllStagedSectors()
}

// Seal all staged sectors
func (sm *StorageMinerAPI) SectorsStagedSeal(context.Context) error {
	return sm.SectorBuilder.SealAllStagedSectors()
}

var _ api.StorageMiner = &StorageMinerAPI{}
