package retrievaladapter

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-car/v2/blockstore"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
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
