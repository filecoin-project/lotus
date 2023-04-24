package shared_testutil

import (
	"github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

type TestStorageBlockstoreAccessor struct {
	Blockstore bstore.Blockstore
}

var _ storagemarket.BlockstoreAccessor = (*TestStorageBlockstoreAccessor)(nil)

func (t *TestStorageBlockstoreAccessor) Get(storagemarket.PayloadCID) (bstore.Blockstore, error) {
	return t.Blockstore, nil
}

func (t *TestStorageBlockstoreAccessor) Done(storagemarket.PayloadCID) error {
	return nil
}

func NewTestStorageBlockstoreAccessor() *TestStorageBlockstoreAccessor {
	return &TestStorageBlockstoreAccessor{
		Blockstore: bstore.NewBlockstore(datastore.NewMapDatastore()),
	}
}

type TestRetrievalBlockstoreAccessor struct {
	Blockstore bstore.Blockstore
}

var _ retrievalmarket.BlockstoreAccessor = (*TestRetrievalBlockstoreAccessor)(nil)

func (t *TestRetrievalBlockstoreAccessor) Get(retrievalmarket.DealID, retrievalmarket.PayloadCID) (bstore.Blockstore, error) {
	return t.Blockstore, nil
}

func (t *TestRetrievalBlockstoreAccessor) Done(retrievalmarket.DealID) error {
	return nil
}

func NewTestRetrievalBlockstoreAccessor() *TestRetrievalBlockstoreAccessor {
	return &TestRetrievalBlockstoreAccessor{
		Blockstore: bstore.NewBlockstore(datastore.NewMapDatastore()),
	}
}
