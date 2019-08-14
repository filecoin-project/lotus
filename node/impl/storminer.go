package impl

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage"
	"github.com/filecoin-project/go-lotus/storage/sector"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.SectorBuilderConfig
	SectorBuilder       *sectorbuilder.SectorBuilder
	Sectors             *sector.Store

	Miner *storage.Miner
}

func (sm *StorageMinerAPI) ActorAddresses(context.Context) ([]address.Address, error) {
	return []address.Address{sm.SectorBuilderConfig.Miner}, nil
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) (uint64, error) {
	size := uint64(1016) // this is the most data we can fit in a 1024 byte sector

	name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
	sectorId, err := sm.Sectors.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), 1016), 0)
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
