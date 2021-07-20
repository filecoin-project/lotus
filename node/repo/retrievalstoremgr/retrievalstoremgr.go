package retrievalstoremgr

import (
	"github.com/filecoin-project/lotus/blockstore"
)

// RetrievalStore references a store for a retrieval deal.
type RetrievalStore interface {
	IsIPFSRetrieval() bool
	Blockstore() blockstore.BasicBlockstore
}

// RetrievalStoreManager manages stores for retrieval deals, abstracting
// the underlying storage mechanism.
type RetrievalStoreManager interface {
	NewStore() (RetrievalStore, error)
	ReleaseStore(RetrievalStore) error
}

// BlockstoreRetrievalStoreManager is a blockstore used for retrieval.
type BlockstoreRetrievalStoreManager struct {
	bs              blockstore.BasicBlockstore
	isIpfsRetrieval bool
}

var _ RetrievalStoreManager = &BlockstoreRetrievalStoreManager{}

// NewBlockstoreRetrievalStoreManager returns a new blockstore based RetrievalStoreManager
func NewBlockstoreRetrievalStoreManager(bs blockstore.BasicBlockstore, isIpfsRetrieval bool) RetrievalStoreManager {
	return &BlockstoreRetrievalStoreManager{
		bs:              bs,
		isIpfsRetrieval: isIpfsRetrieval,
	}
}

// NewStore creates a new store (just uses underlying blockstore)
func (brsm *BlockstoreRetrievalStoreManager) NewStore() (RetrievalStore, error) {
	return &blockstoreRetrievalStore{
		bs:              brsm.bs,
		isIpfsRetrieval: brsm.isIpfsRetrieval,
	}, nil
}

// ReleaseStore for this implementation does nothing
func (brsm *BlockstoreRetrievalStoreManager) ReleaseStore(RetrievalStore) error {
	return nil
}

type blockstoreRetrievalStore struct {
	bs              blockstore.BasicBlockstore
	isIpfsRetrieval bool
}

func (brs *blockstoreRetrievalStore) Blockstore() blockstore.BasicBlockstore {
	return brs.bs
}

func (brs *blockstoreRetrievalStore) IsIPFSRetrieval() bool {
	return brs.isIpfsRetrieval
}
