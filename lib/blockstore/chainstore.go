package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

func NewChainBlockstore() ChainAll {
	return nil
}

type ChainBlockstore interface {
	Has(cid.Cid) (bool, error)
	Get(cid.Cid) (blocks.Block, error)
	GetSize(cid.Cid) (int, error)
	Put(blocks.Block) error
	PutMany([]blocks.Block) error
	DeleteBlock(cid.Cid) error
	AllKeysChan(ctx context.Context) (<-chan cid.Cid, error)
	HashOnRead(enabled bool)
}

type ChainAll interface {
	ChainBlockstore
}
