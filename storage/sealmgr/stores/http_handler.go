package stores

import (
	"io"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/lib/tarutil"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

var log = logging.Logger("stores")

type FetchHandler struct {
	Store
}

func (handler *FetchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { // /storage/
	mux := mux.NewRouter()

	mux.HandleFunc("/{type}/{id}", handler.remoteGetSector).Methods("GET")

	log.Infof("SERVEGETREMOTE %s", r.URL)

	mux.ServeHTTP(w, r)
}

func (handler *FetchHandler) remoteGetSector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id, err := sectorutil.ParseSectorID(vars["id"])
	if err != nil {
		log.Error(err)
		w.WriteHeader(500)
		return
	}

	ft, err := ftFromString(vars["type"])
	if err != nil {
		return
	}
	paths, _, done, err := handler.Store.AcquireSector(r.Context(), id, ft, 0, false)
	if err != nil {
		return
	}
	defer done()

	path := sectorutil.PathByType(paths, ft)
	if path == "" {
		log.Error("acquired path was empty")
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
	if _, err := io.Copy(w, rd); err != nil { // TODO: default 32k buf may be too small
		log.Error(err)
		return
	}
}

func ftFromString(t string) (sectorbuilder.SectorFileType, error) {
	switch t {
	case sectorbuilder.FTUnsealed.String():
		return sectorbuilder.FTUnsealed, nil
	case sectorbuilder.FTSealed.String():
		return sectorbuilder.FTSealed, nil
	case sectorbuilder.FTCache.String():
		return sectorbuilder.FTCache, nil
	default:
		return 0, xerrors.Errorf("unknown sector file type: '%s'", t)
	}
}
