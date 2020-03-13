package impl

import (
	"context"
	"encoding/json"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"
	"net/http"
	"os"
	"strconv"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	storagemarket "github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/go-sectorbuilder"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sealmgr/advmgr"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.Config
	//SectorBuilder       sectorbuilder.Interface
	SectorBlocks *sectorblocks.SectorBlocks

	StorageProvider storagemarket.StorageProvider
	Miner           *storage.Miner
	BlockMiner      *miner.Miner
	Full            api.FullNode
	StorageMgr      *advmgr.Manager `optional:"true"`
}

func (sm *StorageMinerAPI) ServeRemote(w http.ResponseWriter, r *http.Request) {
	if !apistruct.HasPerm(r.Context(), apistruct.PermAdmin) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
		return
	}

	sm.StorageMgr.ServeHTTP(w, r)
}

/*
func (sm *StorageMinerAPI) WorkerStats(context.Context) (sectorbuilder.WorkerStats, error) {
	stat := sm.SectorBuilder.WorkerStats()
	return stat, nil
}*/

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.SectorBuilderConfig.Miner, nil
}

func (sm *StorageMinerAPI) ActorSectorSize(ctx context.Context, addr address.Address) (abi.SectorSize, error) {
	return sm.Full.StateMinerSectorSize(ctx, addr, types.EmptyTSK)
}

func (sm *StorageMinerAPI) PledgeSector(ctx context.Context) error {
	return sm.Miner.PledgeSector()
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid abi.SectorNumber) (api.SectorInfo, error) {
	info, err := sm.Miner.GetSectorInfo(sid)
	if err != nil {
		return api.SectorInfo{}, err
	}

	deals := make([]abi.DealID, len(info.Pieces))
	for i, piece := range info.Pieces {
		if piece.DealID == nil {
			continue
		}
		deals[i] = *piece.DealID
	}

	log := make([]api.SectorLog, len(info.Log))
	for i, l := range info.Log {
		log[i] = api.SectorLog{
			Kind:      l.Kind,
			Timestamp: l.Timestamp,
			Trace:     l.Trace,
			Message:   l.Message,
		}
	}

	return api.SectorInfo{
		SectorID: sid,
		State:    info.State,
		CommD:    info.CommD,
		CommR:    info.CommR,
		Proof:    info.Proof,
		Deals:    deals,
		Ticket:   info.Ticket,
		Seed:     info.Seed,
		Retries:  info.Nonce,

		LastErr: info.LastErr,
		Log:     log,
	}, nil
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]abi.SectorNumber, error) {
	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	out := make([]abi.SectorNumber, len(sectors))
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
		out[strconv.FormatUint(k, 10)] = v
	}

	return out, nil
}

func (sm *StorageMinerAPI) SectorsUpdate(ctx context.Context, id abi.SectorNumber, state api.SectorState) error {
	return sm.Miner.ForceSectorState(ctx, id, state)
}

func (sm *StorageMinerAPI) WorkerConnect(ctx context.Context, url string) error {
	_, err := advmgr.ConnectRemote(ctx, sm.Full, url)
	if err != nil {
		return err
	}

	panic("todo register ")
}

func (sm *StorageMinerAPI) StorageAttach(ctx context.Context, si stores.StorageInfo) error {
	panic("implement me")
}

func (sm *StorageMinerAPI) StorageDeclareSector(ctx context.Context, storageId stores.ID, s abi.SectorID, ft sectorbuilder.SectorFileType) error {
	panic("implement me")
}

func (sm *StorageMinerAPI) StorageFindSector(ctx context.Context, si abi.SectorID, types sectorbuilder.SectorFileType) ([]stores.StorageInfo, error) {
	panic("implement me")
}

func (sm *StorageMinerAPI) MarketImportDealData(ctx context.Context, propCid cid.Cid, path string) error {
	fi, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("failed to open file: %w", err)
	}
	defer fi.Close()

	return sm.StorageProvider.ImportDataForDeal(ctx, propCid, fi)
}

func (sm *StorageMinerAPI) MarketListDeals(ctx context.Context) ([]storagemarket.StorageDeal, error) {
	return sm.StorageProvider.ListDeals(ctx)
}

func (sm *StorageMinerAPI) MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error) {
	return sm.StorageProvider.ListIncompleteDeals()
}

func (sm *StorageMinerAPI) SetPrice(ctx context.Context, p types.BigInt) error {
	return sm.StorageProvider.AddAsk(abi.TokenAmount(p), 60*60*24*100) // lasts for 100 days?
}

func (sm *StorageMinerAPI) DealsList(ctx context.Context) ([]storagemarket.StorageDeal, error) {
	return sm.StorageProvider.ListDeals(ctx)
}

func (sm *StorageMinerAPI) DealsImportData(ctx context.Context, deal cid.Cid, fname string) error {
	fi, err := os.Open(fname)
	if err != nil {
		return xerrors.Errorf("failed to open given file: %w", err)
	}
	defer fi.Close()

	return sm.StorageProvider.ImportDataForDeal(ctx, deal, fi)
}

func (sm *StorageMinerAPI) StorageAddLocal(ctx context.Context, path string) error {
	if sm.StorageMgr == nil {
		return xerrors.Errorf("no storage manager")
	}

	return sm.StorageMgr.AddLocalStorage(path)
}

var _ api.StorageMiner = &StorageMinerAPI{}
