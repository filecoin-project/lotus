package impl

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sector"
	"github.com/filecoin-project/lotus/storage/sectorblocks"

	"golang.org/x/xerrors"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.SectorBuilderConfig
	SectorBuilder       *sectorbuilder.SectorBuilder
	Sectors             *sector.Store
	SectorBlocks        *sectorblocks.SectorBlocks

	Miner *storage.Miner
}

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.SectorBuilderConfig.Miner, nil
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) (uint64, error) {
	ssize, err := sm.Miner.SectorSize(ctx)
	if err != nil {
		return 0, xerrors.Errorf("failed to get miner sector size: %w", err)
	}
	size := sectorbuilder.UserBytesForSectorSize(ssize)

	// TODO: create a deal
	name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
	sectorId, err := sm.Sectors.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), 0)
	if err != nil {
		return 0, err
	}

	return sectorId, err
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid uint64) (sectorbuilder.SectorSealingStatus, error) {
	return sm.SectorBuilder.SealStatus(sid)
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]uint64, error) {
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
