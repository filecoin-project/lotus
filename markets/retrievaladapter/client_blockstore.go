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
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
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

func NewAPIBlockstoreAdapter(sub retrievalmarket.BlockstoreAccessor, apiStoreStates dtypes.ApiBstoreStates) *APIBlockstoreAccessor {
	return &APIBlockstoreAccessor{
		sub:       sub,
		apiStores: apiStoreStates,
	}
}

// APIBlockstoreAccessor adds support to API-specified remote blockstores
type APIBlockstoreAccessor struct {
	sub retrievalmarket.BlockstoreAccessor

	apiStores *statestore.StateStore
}

func (a *APIBlockstoreAccessor) Get(id retrievalmarket.DealID, payloadCID retrievalmarket.PayloadCID) (bstore.Blockstore, error) {
	has, err := a.apiStores.Has(uint64(id))
	if err != nil {
		return nil, xerrors.Errorf("check apiStore exists: %w", err)
	}
	if !has {
		return a.sub.Get(id, payloadCID)
	}

	var ar api.RemoteStore
	if err := a.apiStores.Get(uint64(id)).Get(&ar); err != nil {
		return nil, xerrors.Errorf("getting api store: %w", err)
	}

	return &ar, nil
}

func (a *APIBlockstoreAccessor) Done(id retrievalmarket.DealID) error {
	return a.apiStores.Get(uint64(id)).End()
}

func (a *APIBlockstoreAccessor) Register(id retrievalmarket.DealID, as *api.RemoteStore) error {
	return a.apiStores.Begin(uint64(id), as)
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
