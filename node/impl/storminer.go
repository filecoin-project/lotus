package impl

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	storagemarket "github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/go-sectorbuilder/fs"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/lib/tarutil"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilderConfig *sectorbuilder.Config
	SectorBuilder       sectorbuilder.Interface
	SectorBlocks        *sectorblocks.SectorBlocks

	StorageProvider storagemarket.StorageProvider
	Miner           *storage.Miner
	BlockMiner      *miner.Miner
	Full            api.FullNode
}

func (sm *StorageMinerAPI) ServeRemote(w http.ResponseWriter, r *http.Request) {
	if !apistruct.HasPerm(r.Context(), apistruct.PermAdmin) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
		return
	}

	mux := mux.NewRouter()

	mux.HandleFunc("/remote/{type}/{id}", sm.remoteGetSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}", sm.remotePutSector).Methods("PUT")

	log.Infof("SERVEGETREMOTE %s", r.URL)

	mux.ServeHTTP(w, r)
}

func (sm *StorageMinerAPI) remoteGetSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id, err := strconv.ParseUint(vars["id"], 10, 64)
	if err != nil {
		log.Error("parsing sector id: ", err)
		w.WriteHeader(500)
		return
	}

	path, err := sm.SectorBuilder.SectorPath(fs.DataType(vars["type"]), abi.SectorNumber(id))
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	stat, err := os.Stat(string(path))
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	var rd io.Reader
	if stat.IsDir() {
		rd, err = tarutil.TarDirectory(string(path))
		w.Header().Set("Content-Type", "application/x-tar")
	} else {
		rd, err = os.OpenFile(string(path), os.O_RDONLY, 0644)
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

	id, err := strconv.ParseUint(vars["id"], 10, 64)
	if err != nil {
		log.Error("parsing sector id: ", err)
		w.WriteHeader(500)
		return
	}

	// This is going to get better with worker-to-worker transfers

	path, err := sm.SectorBuilder.SectorPath(fs.DataType(vars["type"]), abi.SectorNumber(id))
	if err != nil {
		if err != fs.ErrNotFound {
			log.Error(err)
			w.WriteHeader(500)
			return
		}

		path, err = sm.SectorBuilder.AllocSectorPath(fs.DataType(vars["type"]), abi.SectorNumber(id), true)
		if err != nil {
			log.Error(err)
			w.WriteHeader(500)
			return
		}
	}

	mediatype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	if err := os.RemoveAll(string(path)); err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	switch mediatype {
	case "application/x-tar":
		if err := tarutil.ExtractTar(r.Body, string(path)); err != nil {
			log.Error(err)
			w.WriteHeader(500)
			return
		}
	default:
		if err := files.WriteTo(files.NewReaderFile(r.Body), string(path)); err != nil {
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

func (sm *StorageMinerAPI) WorkerQueue(ctx context.Context, cfg sectorbuilder.WorkerCfg) (<-chan sectorbuilder.WorkerTask, error) {
	return sm.SectorBuilder.AddWorker(ctx, cfg)
}

func (sm *StorageMinerAPI) WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error {
	return sm.SectorBuilder.TaskDone(ctx, task, res)
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

var _ api.StorageMiner = &StorageMinerAPI{}
