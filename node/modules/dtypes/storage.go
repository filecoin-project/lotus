package dtypes

import (
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	bserv "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-filestore"
	"github.com/ipfs/go-graphsync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	format "github.com/ipfs/go-ipld-format"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-statestore"
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
type ClientDAG format.DAGService
type ClientDealStore *statestore.StateStore
type ClientRequestValidator *requestvalidation.UnifiedRequestValidator
type ClientDatastore datastore.Batching

type Graphsync graphsync.GraphExchange

// ClientDataTransfer is a data transfer manager for the client
type ClientDataTransfer datatransfer.Manager

type ProviderDealStore *statestore.StateStore
type ProviderPieceStore piecestore.PieceStore
type ProviderRequestValidator *requestvalidation.UnifiedRequestValidator

// ProviderDataTransfer is a data transfer manager for the provider
type ProviderDataTransfer datatransfer.Manager

type StagingDAG format.DAGService
type StagingBlockstore blockstore.Blockstore
type StagingGraphsync graphsync.GraphExchange
