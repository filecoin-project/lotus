package impl

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"io"
	"net/http"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.Config
	SectorBuilder       *sectorbuilder.SectorBuilder
	SectorBlocks        *sectorblocks.SectorBlocks

	Miner *storage.Miner
	Full  api.FullNode
}

func (sm *StorageMinerAPI) ServeRemote(w http.ResponseWriter, r *http.Request) {
	if !api.HasPerm(r.Context(), api.PermAdmin) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
		return
	}

	mux := mux.NewRouter()

	mux.HandleFunc("/remote/{type}/{sname}", sm.remoteGetSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{sname}", sm.remotePutSector).Methods("PUT")

	mux.ServeHTTP(w, r)
}

func (sm *StorageMinerAPI) remoteGetSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	fr, err := sm.SectorBuilder.OpenRemoteRead(vars["type"], vars["sname"])
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}
	defer fr.Close()

	w.WriteHeader(200)
	if _, err := io.Copy(w, fr); err != nil {
		log.Error(err)
		return
	}
}

func (sm *StorageMinerAPI) remotePutSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	fr, err := sm.SectorBuilder.OpenRemoteWrite(vars["type"], vars["sname"])
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}
	defer fr.Close()

	w.WriteHeader(200)
	if _, err := io.Copy(w, fr); err != nil {
		log.Error(err)
		return
	}
}

func (sm *StorageMinerAPI) WorkerStats(context.Context) (sectorbuilder.WorkerStats, error) {
	stat := sm.SectorBuilder.WorkerStats()
	return stat, nil
}

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.SectorBuilderConfig.Miner, nil
}

func (sm *StorageMinerAPI) ActorSectorSize(ctx context.Context, addr address.Address) (uint64, error) {
	return sm.Full.StateMinerSectorSize(ctx, addr, nil)
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

func (sm *StorageMinerAPI) WorkerQueue(ctx context.Context) (<-chan sectorbuilder.WorkerTask, error) {
	return sm.SectorBuilder.AddWorker(ctx)
}

func (sm *StorageMinerAPI) WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error {
	return sm.SectorBuilder.TaskDone(ctx, task, res)
}

var _ api.StorageMiner = &StorageMinerAPI{}
