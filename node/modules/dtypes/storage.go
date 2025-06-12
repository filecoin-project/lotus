package dtypes

import (
	bserv "github.com/ipfs/boxo/blockservice"
	exchange "github.com/ipfs/boxo/exchange"
	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/blockstore"
)

// MetadataDS stores metadata. By default it's namespaced under /metadata in
// main repo datastore.
type MetadataDS datastore.Batching

// F3DS stores F3 data. By default it's namespaced under /f3 in main repo datastore.
type F3DS datastore.Batching

type (
	// UniversalBlockstore is the universal blockstore backend.
	UniversalBlockstore blockstore.Blockstore

	// ColdBlockstore is the Cold blockstore abstraction for the splitstore
	ColdBlockstore blockstore.Blockstore

	// HotBlockstore is the Hot blockstore abstraction for the splitstore
	HotBlockstore blockstore.Blockstore

	// SplitBlockstore is the hot/cold blockstore that sits on top of the ColdBlockstore.
	SplitBlockstore blockstore.Blockstore

	// BaseBlockstore is something, coz DI
	BaseBlockstore blockstore.Blockstore

	// BasicChainBlockstore is like ChainBlockstore, but without the optional
	// network fallback support
	BasicChainBlockstore blockstore.Blockstore

	// ChainBlockstore is a blockstore to store chain data (tipsets, blocks,
	// messages). It is physically backed by the BareMonolithBlockstore, but it
	// has a cache on top that is specially tuned for chain data access
	// patterns.
	ChainBlockstore blockstore.Blockstore

	// BasicStateBlockstore is like StateBlockstore, but without the optional
	// network fallback support
	BasicStateBlockstore blockstore.Blockstore

	// StateBlockstore is a blockstore to store state data (state tree). It is
	// physically backed by the BareMonolithBlockstore, but it has a cache on
	// top that is specially tuned for state data access patterns.
	StateBlockstore blockstore.Blockstore

	// ExposedBlockstore is a blockstore that interfaces directly with the
	// network or with users, from which queries are served, and where incoming
	// data is deposited. For security reasons, this store is disconnected from
	// any internal caches. If blocks are added to this store in a way that
	// could render caches dirty (e.g. a block is added when an existence cache
	// holds a 'false' for that block), the process should signal so by calling
	// blockstore.AllCaches.Dirty(cid).
	ExposedBlockstore blockstore.Blockstore
)

type ChainBitswap exchange.Interface
type ChainBlockService bserv.BlockService
