package stores

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	carindex "github.com/ipld/go-car/v2/index"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

type ClosableBlockstore interface {
	bstore.Blockstore
	io.Closer
}

// DAGStoreWrapper hides the details of the DAG store implementation from
// the other parts of go-fil-markets.
type DAGStoreWrapper interface {
	// RegisterShard loads a CAR file into the DAG store and builds an
	// index for it, sending the result on the supplied channel on completion
	RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error

	// LoadShard fetches the data for a shard and provides a blockstore
	// interface to it.
	//
	// The blockstore must be closed to release the shard.
	LoadShard(ctx context.Context, pieceCid cid.Cid) (ClosableBlockstore, error)

	// MigrateDeals migrates the supplied storage deals into the DAG store.
	MigrateDeals(ctx context.Context, deals []storagemarket.MinerDeal) (bool, error)

	// GetPiecesContainingBlock returns the CID of all pieces that contain
	// the block with the given CID
	GetPiecesContainingBlock(blockCID cid.Cid) ([]cid.Cid, error)

	GetIterableIndexForPiece(pieceCid cid.Cid) (carindex.IterableIndex, error)

	// DestroyShard initiates the registration of a new shard.
	//
	// This method returns an error synchronously if preliminary validation fails.
	// Otherwise, it queues the shard for destruction. The caller should monitor
	// supplied channel for a result.
	DestroyShard(ctx context.Context, pieceCid cid.Cid, resch chan dagstore.ShardResult) error

	// Close closes the dag store wrapper.
	Close() error
}

// RegisterShardSync calls the DAGStore RegisterShard method and waits
// synchronously in a dedicated channel until the registration has completed
// fully.
func RegisterShardSync(ctx context.Context, ds DAGStoreWrapper, pieceCid cid.Cid, carPath string, eagerInit bool) error {
	resch := make(chan dagstore.ShardResult, 1)
	if err := ds.RegisterShard(ctx, pieceCid, carPath, eagerInit, resch); err != nil {
		return err
	}

	// TODO: Can I rely on RegisterShard to return an error if the context times out?
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-resch:
		return res.Error
	}
}

// DestroyShardSync calls the DAGStore DestroyShard method and waits
// synchronously in a dedicated channel until the shard has been destroyed completely.
func DestroyShardSync(ctx context.Context, ds DAGStoreWrapper, pieceCid cid.Cid) error {
	resch := make(chan dagstore.ShardResult, 1)

	if err := ds.DestroyShard(ctx, pieceCid, resch); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-resch:
		return res.Error
	}
}
