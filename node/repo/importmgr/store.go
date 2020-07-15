package importmgr

import (
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
)

type Store struct {
	ds datastore.Batching

	fm     *filestore.FileManager
	Fstore *filestore.Filestore

	Bstore blockstore.Blockstore

	bsvc blockservice.BlockService
	DAG  ipld.DAGService
}

func openStore(ds datastore.Batching) (*Store, error) {
	blocks := namespace.Wrap(ds, datastore.NewKey("blocks"))
	bs := blockstore.NewBlockstore(blocks)

	fm := filestore.NewFileManager(ds, "/")
	fm.AllowFiles = true

	fstore := filestore.NewFilestore(bs, fm)
	ibs := blockstore.NewIdStore(fstore)

	bsvc := blockservice.New(ibs, offline.Exchange(ibs))
	dag := merkledag.NewDAGService(bsvc)

	return &Store{
		ds: ds,

		fm:     fm,
		Fstore: fstore,

		Bstore: ibs,

		bsvc: bsvc,
		DAG:  dag,
	}, nil
}

func (s *Store) Close() error {
	return s.bsvc.Close()
}
