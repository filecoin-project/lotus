package retrievalstoremgr

import (
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/ipfs/go-blockservice"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
)

// RetrievalStore references a store for a retrieval deal.
type RetrievalStore interface {
	DAGService() ipldformat.DAGService
}

// RetrievalStoreManager manages stores for retrieval deals, abstracting
// the underlying storage mechanism.
type RetrievalStoreManager interface {
	NewStore() (RetrievalStore, error)
	ReleaseStore(RetrievalStore) error
}

// BlockstoreRetrievalStoreManager manages a single blockstore as if it were multiple stores
type BlockstoreRetrievalStoreManager struct {
	bs blockstore.BasicBlockstore
}

var _ RetrievalStoreManager = &BlockstoreRetrievalStoreManager{}

// NewBlockstoreRetrievalStoreManager returns a new blockstore based RetrievalStoreManager
func NewBlockstoreRetrievalStoreManager(bs blockstore.BasicBlockstore) RetrievalStoreManager {
	return &BlockstoreRetrievalStoreManager{
		bs: bs,
	}
}

// NewStore creates a new store (just uses underlying blockstore)
func (brsm *BlockstoreRetrievalStoreManager) NewStore() (RetrievalStore, error) {
	return &blockstoreRetrievalStore{
		dagService: merkledag.NewDAGService(blockservice.New(brsm.bs, offline.Exchange(brsm.bs))),
	}, nil
}

// ReleaseStore for this implementation does nothing
func (brsm *BlockstoreRetrievalStoreManager) ReleaseStore(RetrievalStore) error {
	return nil
}

type blockstoreRetrievalStore struct {
	dagService ipldformat.DAGService
}

func (brs *blockstoreRetrievalStore) DAGService() ipldformat.DAGService {
	return brs.dagService
}
