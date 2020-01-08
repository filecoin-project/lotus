package dtypes

import (
	bserv "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/lotus/datatransfer"
)

// MetadataDS stores metadata
// dy default it's namespaced under /metadata in main repo datastore
type MetadataDS datastore.Batching

type ChainBlockstore blockstore.Blockstore

type ChainGCLocker blockstore.GCLocker
type ChainGCBlockstore blockstore.GCBlockstore
type ChainExchange exchange.Interface
type ChainBlockService bserv.BlockService

type ClientFilestore *filestore.Filestore
type ClientBlockstore blockstore.Blockstore
type ClientDAG ipld.DAGService
type ClientDealStore *statestore.StateStore

// ClientDataTransfer is a data transfer manager for the client
type ClientDataTransfer datatransfer.Manager

type ProviderDealStore *statestore.StateStore

// ProviderDataTransfer is a data transfer manager for the provider
type ProviderDataTransfer datatransfer.Manager

type StagingDAG ipld.DAGService
