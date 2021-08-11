package storageadapter

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/stores"

	"github.com/filecoin-project/lotus/node/repo/importmgr"
)

// carFileBSAccessor provides access to CARv2 files as blockstores.
// It creates the CAR files in the given baseDir and uses the root
// CID as the filename.
type carFileBSAccessor struct {
	importMgr *importmgr.Mgr

	lk      sync.Mutex
	bstores map[cid.Cid]stores.ClosableBlockstore
}

var _ storageimpl.BlockstoreAccessor = (*carFileBSAccessor)(nil)

func NewCarFileBlockstoreAccessor(importMgr *importmgr.Mgr) *carFileBSAccessor {
	return &carFileBSAccessor{
		importMgr: importMgr,
		bstores:   make(map[cid.Cid]stores.ClosableBlockstore),
	}
}

func (ba *carFileBSAccessor) Get(rootCid cid.Cid) (bstore.Blockstore, error) {
	ba.lk.Lock()
	defer ba.lk.Unlock()

	if bs, ok := ba.bstores[rootCid]; !ok {
		return bs, nil
	}

	carPath, err := ba.importMgr.FilestoreCARV2FilePathFor(rootCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to find CARv2 file path: %w", err)
	}
	if carPath == "" {
		return nil, xerrors.New("no CARv2 file path for deal")
	}

	// Open a read-only blockstore off the CAR file, wrapped in a filestore so
	// it can read file positional references.
	bs, err := stores.ReadOnlyFilestore(carPath)
	if err != nil {
		return nil, xerrors.Errorf("failed to open car filestore: %w", err)
	}

	ba.bstores[rootCid] = bs

	return bs, nil
}

func (ba *carFileBSAccessor) Close(rootCid cid.Cid) error {
	ba.lk.Lock()
	defer ba.lk.Unlock()

	if bs, ok := ba.bstores[rootCid]; !ok {
		delete(ba.bstores, rootCid)
		if err := bs.Close(); err != nil {
			return xerrors.Errorf("failed to close read-only blockstore: %w", err)
		}
	}

	return nil
}

// passThroughBSAccessor implements a BlockstoreAccessor that just passes
// through the underlying monolithic blockstore
type passThroughBSAccessor struct {
	bs bstore.Blockstore
}

var _ storageimpl.BlockstoreAccessor = (*passThroughBSAccessor)(nil)

func NewPassThroughBlockstoreAccessor(bs bstore.Blockstore) *passThroughBSAccessor {
	return &passThroughBSAccessor{bs: bs}
}

func (p *passThroughBSAccessor) Get(rootCid cid.Cid) (bstore.Blockstore, error) {
	return p.bs, nil
}

func (p *passThroughBSAccessor) Close(rootCid cid.Cid) error {
	return nil
}
