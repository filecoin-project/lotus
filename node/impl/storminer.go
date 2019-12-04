package impl

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.Config
	SectorBuilder       *sectorbuilder.SectorBuilder
	SectorBlocks        *sectorblocks.SectorBlocks

	Miner      *storage.Miner
	BlockMiner *miner.Miner
	Full       api.FullNode
}

func (sm *StorageMinerAPI) WorkerStats(context.Context) (api.WorkerStats, error) {
	free, reserved, total := sm.SectorBuilder.WorkerStats()
	return api.WorkerStats{
		Free:     free,
		Reserved: reserved,
		Total:    total,
	}, nil
}

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.SectorBuilderConfig.Miner, nil
}

func (sm *StorageMinerAPI) StoreGarbageData(ctx context.Context) error {
	return sm.Miner.StoreGarbageData()
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid uint64) (api.SectorInfo, error) {
	info, err := sm.Miner.GetSectorInfo(sid)
	if err != nil {
		return api.SectorInfo{}, err
	}

	deals := make([]uint64, len(info.Pieces))
	for i, piece := range info.Pieces {
		deals[i] = piece.DealID
	}

	return api.SectorInfo{
		SectorID: sid,
		State:    info.State,
		CommD:    info.CommD,
		CommR:    info.CommR,
		Proof:    info.Proof,
		Deals:    deals,
		Ticket:   info.Ticket.SB(),
		Seed:     info.Seed.SB(),

		/*LastErr: info.LastErr,*/
	}, nil
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]uint64, error) {
	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	out := make([]uint64, len(sectors))
	for i, sector := range sectors {
		out[i] = sector.SectorID
	}
	return out, nil
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
