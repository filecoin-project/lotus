package stores

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/partialfile"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/extern/sector-storage/tarutil"

	"github.com/filecoin-project/specs-storage/storage"
)

var log = logging.Logger("stores")

type FetchHandler struct {
	*Local
}

func (handler *FetchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { // /remote/
	mux := mux.NewRouter()

	mux.HandleFunc("/remote/stat/{id}", handler.remoteStatFs).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}", handler.remoteGetSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}", handler.remoteDeleteSector).Methods("DELETE")

	mux.HandleFunc("/remote/{type}/{id}/{spt}/allocated/{offset}/{size}", handler.remoteGetAllocated).Methods("GET")

	mux.ServeHTTP(w, r)
}

func (handler *FetchHandler) remoteStatFs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := ID(vars["id"])

	st, err := handler.Local.FsStat(r.Context(), id)
	switch err {
	case errPathNotFound:
		w.WriteHeader(404)
		return
	case nil:
		break
	default:
		w.WriteHeader(500)
		log.Errorf("%+v", err)
		return
	}

	if err := json.NewEncoder(w).Encode(&st); err != nil {
		log.Warnf("error writing stat response: %+v", err)
	}
}

func (handler *FetchHandler) remoteGetSector(w http.ResponseWriter, r *http.Request) {
	log.Infof("SERVE GET %s", r.URL)
	vars := mux.Vars(r)

	id, err := storiface.ParseSectorID(vars["id"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	ft, err := ftFromString(vars["type"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	// The caller has a lock on this sector already, no need to get one here
	// passing 0 spt because we don't allocate anything
	si := storage.SectorRef{
		ID:        id,
		ProofType: 0,
	}

	paths, _, err := handler.Local.AcquireSector(r.Context(), si, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	// TODO: reserve local storage here

	path := storiface.PathByType(paths, ft)
	if path == "" {
		log.Error("acquired path was empty")
		w.WriteHeader(500)
		return
	}

	stat, err := os.Stat(path)
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	if stat.IsDir() {
		if _, has := r.Header["Range"]; has {
			log.Error("Range not supported on directories")
			w.WriteHeader(500)
			return
		}

		rd, err := tarutil.TarDirectory(path)
		if err != nil {
			log.Errorf("%+v", err)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/x-tar")
		w.WriteHeader(200)
		if _, err := io.CopyBuffer(w, rd, make([]byte, CopyBuf)); err != nil {
			log.Errorf("%+v", err)
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path)
	}
}

func (handler *FetchHandler) remoteDeleteSector(w http.ResponseWriter, r *http.Request) {
	log.Infof("SERVE DELETE %s", r.URL)
	vars := mux.Vars(r)

	id, err := storiface.ParseSectorID(vars["id"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	ft, err := ftFromString(vars["type"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	if err := handler.Remove(r.Context(), id, ft, false); err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}
}

func (handler *FetchHandler) remoteGetAllocated(w http.ResponseWriter, r *http.Request) {
	log.Infof("SERVE Alloc check %s", r.URL)
	vars := mux.Vars(r)

	id, err := storiface.ParseSectorID(vars["id"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	ft, err := ftFromString(vars["type"])
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}
	if ft != storiface.FTUnsealed {
		log.Errorf("/allocated only supports unsealed sector files")
		w.WriteHeader(500)
		return
	}

	spti, err := strconv.ParseInt(vars["spt"], 10, 64)
	if err != nil {
		log.Errorf("parsing spt: %+v", err)
		w.WriteHeader(500)
		return
	}
	spt := abi.RegisteredSealProof(spti)
	ssize, err := spt.SectorSize()
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	offi, err := strconv.ParseInt(vars["offset"], 10, 64)
	if err != nil {
		log.Errorf("parsing offset: %+v", err)
		w.WriteHeader(500)
		return
	}
	szi, err := strconv.ParseInt(vars["size"], 10, 64)
	if err != nil {
		log.Errorf("parsing spt: %+v", err)
		w.WriteHeader(500)
		return
	}

	// The caller has a lock on this sector already, no need to get one here

	// passing 0 spt because we don't allocate anything
	si := storage.SectorRef{
		ID:        id,
		ProofType: 0,
	}

	paths, _, err := handler.Local.AcquireSector(r.Context(), si, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}

	path := storiface.PathByType(paths, ft)
	if path == "" {
		log.Error("acquired path was empty")
		w.WriteHeader(500)
		return
	}

	pf, err := partialfile.OpenPartialFile(abi.PaddedPieceSize(ssize), path)
	if err != nil {
		log.Error("opening partial file: ", err)
		w.WriteHeader(500)
		return
	}
	defer func() {
		if err := pf.Close(); err != nil {
			log.Error("close partial file: ", err)
		}
	}()

	has, err := pf.HasAllocated(storiface.UnpaddedByteIndex(offi), abi.UnpaddedPieceSize(szi))
	if err != nil {
		log.Error("has allocated: ", err)
		w.WriteHeader(500)
		return
	}

	if has {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
}

func ftFromString(t string) (storiface.SectorFileType, error) {
	switch t {
	case storiface.FTUnsealed.String():
		return storiface.FTUnsealed, nil
	case storiface.FTSealed.String():
		return storiface.FTSealed, nil
	case storiface.FTCache.String():
		return storiface.FTCache, nil
	default:
		return 0, xerrors.Errorf("unknown sector file type: '%s'", t)
	}
}
