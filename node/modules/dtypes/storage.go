package dtypes

import (
	bserv "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-graphsync"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	format "github.com/ipfs/go-ipld-format"

	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	"github.com/filecoin-project/go-multistore"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/filecoin-project/lotus/node/repo/retrievalstoremgr"
)

// MetadataDS stores metadata. By default it's namespaced under /metadata in
// main repo datastore.
type MetadataDS datastore.Batching

type (
	// BareMonolithBlockstore is the current monolithic blockstore as opened from
	// the filesystem, with no caching on top. It may be overriden by a
	// Bitswap-fallback if that setting is enabled.
	BareMonolithBlockstore blockstore.Blockstore

	// ChainBlockstore is a blockstore to store chain data. It is currently a
	// synonym of the BareMonolithBlockstore, but it is fronted by a dedicated
	// chain data cache.
	ChainBlockstore blockstore.Blockstore

	// StateBlockstore is a blockstore to store state data. It is currently a
	// synonym of the BareMonolithiBlockstore, but it is frontend by a dedicated
	// state data cache.
	StateBlockstore blockstore.Blockstore

	// ExposedBlockstore is the blockstore that's safe to expose externally, useful
	// when exchanging blobs over the network, such as when using Bitswap,
	// Graphsync, and the JSON-RPC APIs. Operations on this store do not affect the
	// caches, and thus prevents untrusted parties from affecting the cache.
	ExposedBlockstore blockstore.Blockstore
)

type ChainBitswap exchange.Interface
type ChainBlockService bserv.BlockService

type ClientMultiDstore *multistore.MultiStore
type ClientImportMgr *importmgr.Mgr
type ClientBlockstore blockstore.Blockstore
type ClientDealStore *statestore.StateStore
type ClientRequestValidator *requestvalidation.UnifiedRequestValidator
type ClientDatastore datastore.Batching
type ClientRetrievalStoreManager retrievalstoremgr.RetrievalStoreManager

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
type StagingMultiDstore *multistore.MultiStore
