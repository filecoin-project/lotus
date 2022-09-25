package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
)

type apiBstoreServer struct {
	urlPrefix string // ends with a /

	stores map[uuid.UUID]blockstore.Blockstore

	lk sync.RWMutex
}

func (a *apiBstoreServer) MakeRemoteBstore(bs blockstore.Blockstore) api.RemoteStore {
	a.lk.Lock()
	defer a.lk.Unlock()

	id := uuid.New()
	a.stores[id] = bs

	return api.RemoteStore{
		PutURL: api.URLTemplate{
			UrlTemplate: a.urlPrefix + fmt.Sprintf("/put?store=%s&cid={{.cid}}", id),
			Headers:     nil,
		},
	}
}

// todo free store

func (a *apiBstoreServer) ServePut(w http.ResponseWriter, r *http.Request) {
	a.lk.RLock()
	defer a.lk.RUnlock()

	c, err := cid.Parse(r.FormValue("cid"))
	if err != nil {
		http.Error(w, xerrors.Errorf("parsing cid: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	blkData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	chkc, err := c.Prefix().Sum(blkData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !chkc.Equals(c) {
		http.Error(w, "bad block data", http.StatusInternalServerError)
		return
	}

	bb, err := block.NewBlockWithCid(blkData, c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	storeID, err := uuid.Parse(r.FormValue("store"))
	if err != nil {
		http.Error(w, xerrors.Errorf("parsing store id: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	st, found := a.stores[storeID]
	if !found {
		http.Error(w, "store not found", http.StatusInternalServerError)
		return
	}

	if err := st.Put(r.Context(), bb); err != nil {
		http.Error(w, xerrors.Errorf("put: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
