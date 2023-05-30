package retrievaladapter

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"

	"github.com/filecoin-project/lotus/api"
	lbstore "github.com/filecoin-project/lotus/blockstore"
)

// ProxyBlockstoreAccessor is an accessor that returns a fixed blockstore.
// To be used in combination with IPFS integration.
type ProxyBlockstoreAccessor struct {
	Blockstore bstore.Blockstore
}

var _ retrievalmarket.BlockstoreAccessor = (*ProxyBlockstoreAccessor)(nil)

func NewFixedBlockstoreAccessor(bs bstore.Blockstore) retrievalmarket.BlockstoreAccessor {
	return &ProxyBlockstoreAccessor{Blockstore: bs}
}

func (p *ProxyBlockstoreAccessor) Get(_ retrievalmarket.DealID, _ retrievalmarket.PayloadCID) (bstore.Blockstore, error) {
	return p.Blockstore, nil
}

func (p *ProxyBlockstoreAccessor) Done(_ retrievalmarket.DealID) error {
	return nil
}

func NewAPIBlockstoreAdapter(sub retrievalmarket.BlockstoreAccessor) *APIBlockstoreAccessor {
	return &APIBlockstoreAccessor{
		sub:          sub,
		retrStores:   map[retrievalmarket.DealID]api.RemoteStoreID{},
		remoteStores: map[api.RemoteStoreID]bstore.Blockstore{},
	}
}

// APIBlockstoreAccessor adds support to API-specified remote blockstores
type APIBlockstoreAccessor struct {
	sub retrievalmarket.BlockstoreAccessor

	retrStores   map[retrievalmarket.DealID]api.RemoteStoreID
	remoteStores map[api.RemoteStoreID]bstore.Blockstore

	accessLk sync.Mutex
}

func (a *APIBlockstoreAccessor) Get(id retrievalmarket.DealID, payloadCID retrievalmarket.PayloadCID) (bstore.Blockstore, error) {
	a.accessLk.Lock()
	defer a.accessLk.Unlock()

	as, has := a.retrStores[id]
	if !has {
		return a.sub.Get(id, payloadCID)
	}

	return a.remoteStores[as], nil
}

func (a *APIBlockstoreAccessor) Done(id retrievalmarket.DealID) error {
	a.accessLk.Lock()
	defer a.accessLk.Unlock()

	if _, has := a.retrStores[id]; has {
		delete(a.retrStores, id)
		return nil
	}
	return a.sub.Done(id)
}

func (a *APIBlockstoreAccessor) RegisterDealToRetrievalStore(id retrievalmarket.DealID, sid api.RemoteStoreID) error {
	a.accessLk.Lock()
	defer a.accessLk.Unlock()

	if _, has := a.retrStores[id]; has {
		return xerrors.Errorf("apistore for deal %d already registered", id)
	}
	if _, has := a.remoteStores[sid]; !has {
		return xerrors.Errorf("remote store not found")
	}

	a.retrStores[id] = sid
	return nil
}

func (a *APIBlockstoreAccessor) RegisterApiStore(sid api.RemoteStoreID, st *lbstore.NetworkStore) error {
	a.accessLk.Lock()
	defer a.accessLk.Unlock()

	if _, has := a.remoteStores[sid]; has {
		return xerrors.Errorf("remote store already registered with this uuid")
	}

	a.remoteStores[sid] = st

	st.OnClose(func() {
		a.accessLk.Lock()
		defer a.accessLk.Unlock()

		if _, has := a.remoteStores[sid]; has {
			delete(a.remoteStores, sid)
		}
	})
	return nil
}

var _ retrievalmarket.BlockstoreAccessor = &APIBlockstoreAccessor{}

type CARBlockstoreAccessor struct {
	rootdir string
	lk      sync.Mutex
	open    map[retrievalmarket.DealID]*blockstore.ReadWrite
}

var _ retrievalmarket.BlockstoreAccessor = (*CARBlockstoreAccessor)(nil)

func NewCARBlockstoreAccessor(rootdir string) *CARBlockstoreAccessor {
	return &CARBlockstoreAccessor{
		rootdir: rootdir,
		open:    make(map[retrievalmarket.DealID]*blockstore.ReadWrite),
	}
}

func (c *CARBlockstoreAccessor) Get(id retrievalmarket.DealID, payloadCid retrievalmarket.PayloadCID) (bstore.Blockstore, error) {
	c.lk.Lock()
	defer c.lk.Unlock()

	bs, ok := c.open[id]
	if ok {
		return bs, nil
	}

	path := c.PathFor(id)
	bs, err := blockstore.OpenReadWrite(path, []cid.Cid{payloadCid}, blockstore.UseWholeCIDs(true))
	if err != nil {
		return nil, err
	}
	c.open[id] = bs
	return bs, nil
}

func (c *CARBlockstoreAccessor) Done(id retrievalmarket.DealID) error {
	c.lk.Lock()
	defer c.lk.Unlock()

	bs, ok := c.open[id]
	if !ok {
		return nil
	}

	delete(c.open, id)
	return bs.Finalize()
}

func (c *CARBlockstoreAccessor) PathFor(id retrievalmarket.DealID) string {
	return filepath.Join(c.rootdir, fmt.Sprintf("%d.car", id))
}
