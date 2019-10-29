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
	"github.com/filecoin-project/lotus/storage/commitment"
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
	CommitmentTracker   *commitment.Tracker

	Miner *storage.Miner
}

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.SectorBuilderConfig.Miner, nil
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) error {
	ssize, err := sm.Miner.SectorSize(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get miner sector size: %w", err)
	}
	go func() {
		size := sectorbuilder.UserBytesForSectorSize(ssize)

		// TODO: create a deal
		name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
		sectorId, err := sm.Sectors.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)))
		if err != nil {
			log.Error(err)
			return
		}

		if err := sm.Sectors.SealSector(ctx, sectorId); err != nil {
			log.Error(err)
			return
		}
	}()

	return err
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid uint64) (sectorbuilder.SectorSealingStatus, error) {
	return sm.SectorBuilder.SealStatus(sid)
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]uint64, error) {
	return sm.SectorBuilder.GetAllStagedSectors()
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

func (sm *StorageMinerAPI) CommitmentsList(ctx context.Context) ([]api.SectorCommitment, error) {
	return sm.CommitmentTracker.List()
}

var _ api.StorageMiner = &StorageMinerAPI{}
