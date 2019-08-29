package impl

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage"
	"github.com/filecoin-project/go-lotus/storage/sector"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.SectorBuilderConfig
	SectorBuilder       *sectorbuilder.SectorBuilder
	Sectors             *sector.Store
	SectorBlocks        *sectorblocks.SectorBlocks

	Miner *storage.Miner
}

func (sm *StorageMinerAPI) ActorAddresses(context.Context) ([]address.Address, error) {
	return []address.Address{sm.SectorBuilderConfig.Miner}, nil
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) (uint64, error) {
	size := sectorbuilder.UserBytesForSectorSize(build.SectorSize)

	name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
	sectorId, err := sm.Sectors.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), 1016))
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

func (sm *StorageMinerAPI) SectorsRefs(context.Context) (map[string][]api.SealedRef, error) {
	// json can't handle cids as map keys
	out := map[string][]api.SealedRef{}

	refs, err := sm.SectorBlocks.List()
	if err != nil {
		return nil, err
	}

	for k, v := range refs {
		out[k.String()] = v
	}

	return out, nil
}

var _ api.StorageMiner = &StorageMinerAPI{}
