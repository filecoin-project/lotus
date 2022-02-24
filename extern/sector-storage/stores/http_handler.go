package stores

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/partialfile"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/extern/sector-storage/tarutil"

	"github.com/filecoin-project/specs-storage/storage"
)

var log = logging.Logger("stores")

var _ PartialFileHandler = &DefaultPartialFileHandler{}

// DefaultPartialFileHandler is the default implementation of the PartialFileHandler interface.
// This is probably the only implementation we'll ever use because the purpose of the
// interface to is to mock out partial file related functionality during testing.
type DefaultPartialFileHandler struct{}

func (d *DefaultPartialFileHandler) OpenPartialFile(maxPieceSize abi.PaddedPieceSize, path string) (*partialfile.PartialFile, error) {
	return partialfile.OpenPartialFile(maxPieceSize, path)
}
func (d *DefaultPartialFileHandler) HasAllocated(pf *partialfile.PartialFile, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	return pf.HasAllocated(offset, size)
}

func (d *DefaultPartialFileHandler) Reader(pf *partialfile.PartialFile, offset storiface.PaddedByteIndex, size abi.PaddedPieceSize) (*os.File, error) {
	return pf.Reader(offset, size)
}

// Close closes the partial file
func (d *DefaultPartialFileHandler) Close(pf *partialfile.PartialFile) error {
	return pf.Close()
}

type FetchHandler struct {
	Local     Store
	PfHandler PartialFileHandler
}

func (handler *FetchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { // /remote/
	mux := mux.NewRouter()

	mux.HandleFunc("/remote/stat/{id}", handler.remoteStatFs).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}/{spt}/allocated/{offset}/{size}", handler.remoteGetAllocated).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}", handler.remoteGetSector).Methods("GET")
	mux.HandleFunc("/remote/{type}/{id}", handler.remoteDeleteSector).Methods("DELETE")

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

// remoteGetSector returns the sector file/tared directory byte stream for the sectorID and sector file type sent in the request.
// returns an error if it does NOT have the required sector file/dir.
func (handler *FetchHandler) remoteGetSector(w http.ResponseWriter, r *http.Request) {
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
		log.Errorf("AcquireSector: %+v", err)
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
		log.Errorf("os.Stat: %+v", err)
		w.WriteHeader(500)
		return
	}

	if stat.IsDir() {
		if _, has := r.Header["Range"]; has {
			log.Error("Range not supported on directories")
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/x-tar")
		w.WriteHeader(200)

		err := tarutil.TarDirectory(path, w, make([]byte, CopyBuf))
		if err != nil {
			log.Errorf("send tar: %+v", err)
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		// will do a ranged read over the file at the given path if the caller has asked for a ranged read in the request headers.
		http.ServeFile(w, r, path)
	}

	log.Debugf("served sector file/dir, sectorID=%+v, fileType=%s, path=%s", id, ft, path)
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

	if err := handler.Local.Remove(r.Context(), id, ft, false, []ID{ID(r.FormValue("keep"))}); err != nil {
		log.Errorf("%+v", err)
		w.WriteHeader(500)
		return
	}
}

// remoteGetAllocated returns `http.StatusOK` if the worker already has an Unsealed sector file
// containing the Unsealed piece sent in the request.
// returns `http.StatusRequestedRangeNotSatisfiable` otherwise.
func (handler *FetchHandler) remoteGetAllocated(w http.ResponseWriter, r *http.Request) {
	log.Infof("SERVE Alloc check %s", r.URL)
	vars := mux.Vars(r)

	id, err := storiface.ParseSectorID(vars["id"])
	if err != nil {
		log.Errorf("parsing sectorID: %+v", err)
		w.WriteHeader(500)
		return
	}

	ft, err := ftFromString(vars["type"])
	if err != nil {
		log.Errorf("ftFromString: %+v", err)
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
		log.Errorf("spt.SectorSize(): %+v", err)
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
		log.Errorf("parsing size: %+v", err)
		w.WriteHeader(500)
		return
	}

	// The caller has a lock on this sector already, no need to get one here

	// passing 0 spt because we don't allocate anything
	si := storage.SectorRef{
		ID:        id,
		ProofType: 0,
	}

	// get the path of the local Unsealed file for the given sector.
	// return error if we do NOT have it.
	paths, _, err := handler.Local.AcquireSector(r.Context(), si, ft, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
	if err != nil {
		log.Errorf("AcquireSector: %+v", err)
		w.WriteHeader(500)
		return
	}

	path := storiface.PathByType(paths, ft)
	if path == "" {
		log.Error("acquired path was empty")
		w.WriteHeader(500)
		return
	}

	// open the Unsealed file and check if it has the Unsealed sector for the piece at the given offset and size.
	pf, err := handler.PfHandler.OpenPartialFile(abi.PaddedPieceSize(ssize), path)
	if err != nil {
		log.Error("opening partial file: ", err)
		w.WriteHeader(500)
		return
	}
	defer func() {
		if err := pf.Close(); err != nil {
			log.Error("closing partial file: ", err)
		}
	}()

	has, err := handler.PfHandler.HasAllocated(pf, storiface.UnpaddedByteIndex(offi), abi.UnpaddedPieceSize(szi))
	if err != nil {
		log.Error("has allocated: ", err)
		w.WriteHeader(500)
		return
	}

	if has {
		log.Debugf("returning ok: worker has unsealed file with unsealed piece, sector:%+v, offset:%d, size:%d", id, offi, szi)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Debugf("returning StatusRequestedRangeNotSatisfiable: worker does NOT have unsealed file with unsealed piece, sector:%+v, offset:%d, size:%d", id, offi, szi)
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
	case storiface.FTUpdate.String():
		return storiface.FTUpdate, nil
	case storiface.FTUpdateCache.String():
		return storiface.FTUpdateCache, nil
	default:
		return 0, xerrors.Errorf("unknown sector file type: '%s'", t)
	}
}
