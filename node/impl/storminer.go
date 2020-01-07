package impl

import (
	"context"
	"encoding/json"
	"github.com/filecoin-project/lotus/api/apistruct"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	files "github.com/ipfs/go-ipfs-files"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/tarutil"
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

func (sm *StorageMinerAPI) ServeRemote(w http.ResponseWriter, r *http.Request) {
	if !apistruct.HasPerm(r.Context(), apistruct.PermAdmin) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
		return
	}

	mux := mux.NewRouter()

	mux.HandleFunc("/remote/{type}/{sname}", sm.remoteGetSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{sname}", sm.remotePutSector).Methods("PUT")

	log.Infof("SERVEGETREMOTE %s", r.URL)

	mux.ServeHTTP(w, r)
}

func (sm *StorageMinerAPI) remoteGetSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	path, err := sm.SectorBuilder.GetPath(vars["type"], vars["sname"])
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	stat, err := os.Stat(path)
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	var rd io.Reader
	if stat.IsDir() {
		rd, err = tarutil.TarDirectory(path)
		w.Header().Set("Content-Type", "application/x-tar")
	} else {
		rd, err = os.OpenFile(path, os.O_RDONLY, 0644)
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	w.WriteHeader(200)
	if _, err := io.Copy(w, rd); err != nil {
		log.Error(err)
		return
	}
}

func (sm *StorageMinerAPI) remotePutSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	path, err := sm.SectorBuilder.GetPath(vars["type"], vars["sname"])
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	mediatype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	if err := os.RemoveAll(path); err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	switch mediatype {
	case "application/x-tar":
		if err := tarutil.ExtractTar(r.Body, path); err != nil {
			log.Error(err)
			w.WriteHeader(500)
			return
		}
	default:
		if err := files.WriteTo(files.NewReaderFile(r.Body), path); err != nil {
			log.Error(err)
			w.WriteHeader(500)
			return
		}
	}

	w.WriteHeader(200)

	log.Infof("received %s sector (%s): %d bytes", vars["type"], vars["sname"], r.ContentLength)
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

func (sm *StorageMinerAPI) PledgeSector(ctx context.Context) error {
	return sm.Miner.PledgeSector()
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
		Retries:  info.Nonce,

		LastErr: info.LastErr,
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

func (sm *StorageMinerAPI) SectorsUpdate(ctx context.Context, id uint64, state api.SectorState) error {
	return sm.Miner.UpdateSectorState(ctx, id, storage.NonceIncrement, state)
}

func (sm *StorageMinerAPI) WorkerQueue(ctx context.Context, cfg sectorbuilder.WorkerCfg) (<-chan sectorbuilder.WorkerTask, error) {
	return sm.SectorBuilder.AddWorker(ctx, cfg)
}

func (sm *StorageMinerAPI) WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error {
	return sm.SectorBuilder.TaskDone(ctx, task, res)
}

var _ api.StorageMiner = &StorageMinerAPI{}
